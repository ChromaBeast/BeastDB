package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestSettingsReportsEncryptionFromEngineState(t *testing.T) {
	sm := auth.NewSessionManager(make([]byte, 32))
	get := func(enc api.EncryptionInfo) map[string]any {
		h := SettingsHandler(sm, RuntimeSettings{Role: "leader", PoolSize: 8, Encryption: enc})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, sessionReq(t, sm, auth.RoleAdmin, "/api/settings"))
		var got struct {
			Storage map[string]any `json:"storage"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got.Storage
	}
	off := get(api.EncryptionInfo{})
	if off["encryption_at_rest"] != false {
		t.Fatalf("plaintext engine must report off: %v", off)
	}
	if off["audit_log_encrypted"] != false || off["encryption_key_source"] != "" {
		t.Fatalf("plaintext engine must not report a key source or a sealed audit log: %v", off)
	}
	on := get(api.EncryptionInfo{Enabled: true, Cipher: "aes-256-gcm", KeyID: 3, KeySource: "exec"})
	if on["encryption_at_rest"] != true || on["encryption_cipher"] != "aes-256-gcm" || on["encryption_key_id"] != float64(3) || on["encryption_key_source"] != "exec" {
		t.Fatalf("encrypted engine: %v", on)
	}
}
