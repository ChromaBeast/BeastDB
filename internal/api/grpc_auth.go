package api

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TokenValidator evaluates whether an API token is valid against the database.
type TokenValidator interface {
	Validate(raw string) bool
}

// RoleTokenValidator is an optional extension of TokenValidator that also
// reports the role bound to a valid token. Validators that implement it get
// per-method authorization; plain TokenValidators are treated as admin
// (their historical behavior).
type RoleTokenValidator interface {
	TokenValidator
	ValidateRole(raw string) (role string, ok bool)
}

// Roles understood by the gRPC authorizer.
const (
	RoleAdmin       = "admin"
	RoleWriter      = "writer"
	RoleViewer      = "viewer"
	RoleReplication = "replication"
)

// ValidTokenRole reports whether role is a recognized API token role.
func ValidTokenRole(role string) bool {
	switch role {
	case RoleAdmin, RoleWriter, RoleViewer, RoleReplication:
		return true
	}
	return false
}

// methodRoles is the deny-by-default method -> allowed roles map. Methods not
// listed here are rejected for every caller.
var methodRoles = map[string][]string{
	"/beastdb.v1.BeastDBService/Get":           {RoleViewer, RoleWriter, RoleAdmin},
	"/beastdb.v1.BeastDBService/Scan":          {RoleViewer, RoleWriter, RoleAdmin},
	"/beastdb.v1.BeastDBService/Put":           {RoleWriter, RoleAdmin},
	"/beastdb.v1.BeastDBService/Delete":        {RoleWriter, RoleAdmin},
	"/beastdb.v1.BeastDBService/BatchWrite":    {RoleWriter, RoleAdmin},
	"/beastdb.v1.ReplicationService/StreamWAL": {RoleReplication, RoleAdmin},
	"/beastdb.v1.ReplicationService/Ack":       {RoleReplication, RoleAdmin},
}

func roleAllowed(role, method string) bool {
	for _, r := range methodRoles[method] {
		if r == role {
			return true
		}
	}
	return false
}

// AuthInterceptor enforces Bearer token authentication on incoming gRPC RPCs.
type AuthInterceptor struct {
	staticToken string
	validator   TokenValidator
}

// NewAuthInterceptor creates a new gRPC auth interceptor with optional dynamic validator.
func NewAuthInterceptor(staticToken string, validator ...TokenValidator) *AuthInterceptor {
	var v TokenValidator
	if len(validator) > 0 {
		v = validator[0]
	}
	return &AuthInterceptor{
		staticToken: strings.TrimSpace(staticToken),
		validator:   v,
	}
}

// Unary returns a grpc.UnaryServerInterceptor.
func (a *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := a.authorize(ctx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// Stream returns a grpc.StreamServerInterceptor.
func (a *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := a.authorize(ss.Context(), info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func (a *AuthInterceptor) authorize(ctx context.Context, fullMethod string) error {
	if a.staticToken == "" && a.validator == nil {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "missing metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return status.Errorf(codes.Unauthenticated, "missing authorization header")
	}
	header := vals[0]
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return status.Errorf(codes.Unauthenticated, "authorization must be Bearer token")
	}
	token := strings.TrimSpace(header[7:])
	role := ""
	if a.staticToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(a.staticToken)) == 1 {
		role = RoleAdmin
	} else if rv, ok := a.validator.(RoleTokenValidator); ok && rv != nil {
		if r, valid := rv.ValidateRole(token); valid {
			role = r
		}
	} else if a.validator != nil && a.validator.Validate(token) {
		role = RoleAdmin // legacy validator without roles
	}
	if role == "" {
		return status.Errorf(codes.Unauthenticated, "invalid authorization token")
	}
	if !roleAllowed(role, fullMethod) {
		return status.Errorf(codes.PermissionDenied, "role %q is not permitted to call %s", role, fullMethod)
	}
	return nil
}
