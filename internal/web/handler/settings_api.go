package handler

import (
	"net/http"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// RuntimeSettings is the non-secret startup configuration shown read-only in
// the Studio. It must never carry secret values: secrets appear only as
// booleans (see SettingsSecurity).
type RuntimeSettings struct {
	Role                 string
	Version              string
	GRPCAddr             string
	WebAddr              string
	DataDir              string
	PoolSize             int
	WALCheckpointMB      int
	ReplicaID            string
	LeaderAddr           string
	TLSEnabled           bool
	MutualTLS            bool
	APITokenConfigured   bool
	MetricsTokenSet      bool
	PartitionConfigPath  string
	DevMode              bool
	InsecureAuthOverride bool
}

type settingsResponse struct {
	Node     map[string]any `json:"node"`
	Storage  map[string]any `json:"storage"`
	Network  map[string]any `json:"network"`
	Security map[string]any `json:"security"`
	Limits   map[string]any `json:"limits"`
	// Note states how settings change; the Studio shows it verbatim.
	Note string `json:"note"`
}

// SettingsHandler serves GET /api/settings (admin session required).
func SettingsHandler(sessions *auth.SessionManager, s RuntimeSettings) http.Handler {
	resp := buildSettings(s)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, role, err := sessions.ValidateSession(r); err != nil || role != auth.RoleAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, resp)
	})
}

func buildSettings(s RuntimeSettings) settingsResponse {
	return settingsResponse{
		Node: map[string]any{
			"role":       s.Role,
			"version":    s.Version,
			"replica_id": s.ReplicaID,
			"leader_addr": func() string {
				if s.Role == "leader" {
					return ""
				}
				return s.LeaderAddr
			}(),
			"dev_mode": s.DevMode,
		},
		Storage: map[string]any{
			"data_dir":          s.DataDir,
			"pool_size_frames":  s.PoolSize,
			"page_size_bytes":   4096,
			"wal_checkpoint_mb": s.WALCheckpointMB,
		},
		Network: map[string]any{
			"grpc_addr": s.GRPCAddr,
			"web_addr":  s.WebAddr,
		},
		Security: map[string]any{
			"tls_enabled":              s.TLSEnabled,
			"mutual_tls":               s.MutualTLS,
			"api_token_configured":     s.APITokenConfigured,
			"metrics_token_configured": s.MetricsTokenSet,
			"insecure_auth_override":   s.InsecureAuthOverride,
			"partition_config_path":    s.PartitionConfigPath,
		},
		Limits: map[string]any{
			"max_value_bytes":       api.MaxValueSize,
			"max_inline_record":     api.MaxRecordSize,
			"max_batch_bytes":       api.MaxBatchBytes,
			"max_grpc_message":      api.MaxGRPCMessageSize,
			"max_web_put_body":      maxPutBodySize,
			"max_records_page_size": 200,
		},
		Note: "Settings are fixed at startup. Change them with command-line flags or BEASTDB_* environment variables and restart the node.",
	}
}
