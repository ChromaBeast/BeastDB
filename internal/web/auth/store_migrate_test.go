package auth

import "testing"

func TestUserRecordsLiveInSystemPartition(t *testing.T) {
	if got := uint8(sysUserKey("admin") >> 56); got != TokenPartitionPrefix {
		t.Fatalf("user key partition = %#x, want %#x", got, TokenPartitionPrefix)
	}
}

func TestMigrateLegacyUser(t *testing.T) {
	eng := newMockEngine()
	legacy := &User{Username: "admin", PasswordHash: "h", Role: RoleAdmin}
	data, _ := legacy.Marshal()
	_ = eng.Put(legacyUserKey("admin"), data)
	s := NewUserStore(eng)

	moved, err := s.MigrateLegacyUser("admin")
	if err != nil || !moved {
		t.Fatalf("migrate: moved=%v err=%v", moved, err)
	}
	if _, ok := eng.data[legacyUserKey("admin")]; ok {
		t.Fatalf("legacy record was left behind")
	}
	if u, err := s.GetUser("admin"); err != nil || u.PasswordHash != "h" {
		t.Fatalf("migrated user not readable: %v", err)
	}
	if moved, _ := s.MigrateLegacyUser("admin"); moved {
		t.Fatalf("second migration should be a no-op")
	}
}

// A record planted at the legacy key after migration must never be honoured.
func TestLegacyKeyIsNotUsedForLogin(t *testing.T) {
	eng := newMockEngine()
	s := NewUserStore(eng)
	planted := &User{Username: "evil", PasswordHash: "h", Role: RoleAdmin}
	data, _ := planted.Marshal()
	_ = eng.Put(legacyUserKey("evil"), data)
	if _, err := s.GetUser("evil"); err == nil {
		t.Fatalf("user store read a record from the client keyspace")
	}
}
