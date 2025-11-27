package domain

import (
	"testing"
)

func TestCredentialsValidation(t *testing.T) {
	tests := []struct {
		name    string
		creds   *CredentialsInput
		wantErr bool
	}{
		{
			name:    "valid credentials",
			creds:   &CredentialsInput{Username: "user@example.com", Password: "password123"},
			wantErr: false,
		},
		{
			name:    "empty username",
			creds:   &CredentialsInput{Username: "", Password: "password123"},
			wantErr: true,
		},
		{
			name:    "empty password",
			creds:   &CredentialsInput{Username: "user@example.com", Password: ""},
			wantErr: true,
		},
		{
			name:    "username too long",
			creds:   &CredentialsInput{Username: string(make([]byte, 101)), Password: "password123"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.creds.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		server  *ServerInput
		wantErr bool
	}{
		{
			name:    "valid server input",
			server:  &ServerInput{Host: "192.168.1.100", Port: "32400", AccessToken: "abc123def456ghi789jkl"},
			wantErr: false,
		},
		{
			name:    "empty host",
			server:  &ServerInput{Host: "", Port: "32400", AccessToken: "abc123def456ghi789jkl"},
			wantErr: true,
		},
		{
			name:    "port with letters",
			server:  &ServerInput{Host: "192.168.1.100", Port: "3240a", AccessToken: "abc123def456ghi789jkl"},
			wantErr: true,
		},
		{
			name:    "invalid token (too short)",
			server:  &ServerInput{Host: "192.168.1.100", Port: "32400", AccessToken: "short"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.server.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPlexTokenValidation(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid token",
			token:   "abc123def456ghi789jkl0123456789",
			wantErr: false,
		},
		{
			name:    "token with dash",
			token:   "abc-123-def-456-ghi-789-jkl-0123",
			wantErr: false,
		},
		{
			name:    "token with underscore",
			token:   "abc_123_def_456_ghi_789_jkl_0123",
			wantErr: false,
		},
		{
			name:    "token too short",
			token:   "abc123",
			wantErr: true,
		},
		{
			name:    "token with invalid characters",
			token:   "abc@123def456ghi789jkl",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateVar(tt.token, "plextoken")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVar() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPlexHostValidation(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{
			name:    "localhost",
			host:    "localhost",
			wantErr: false,
		},
		{
			name:    "IP address",
			host:    "192.168.1.100",
			wantErr: false,
		},
		{
			name:    "IP with port",
			host:    "192.168.1.100:32400",
			wantErr: false,
		},
		{
			name:    "hostname",
			host:    "plex.example.com",
			wantErr: false,
		},
		{
			name:    "empty host",
			host:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateVar(tt.host, "plexhost")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVar() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	creds := &CredentialsInput{Username: "", Password: ""}
	err := creds.Validate()

	if err == nil {
		t.Fatal("expected validation error")
	}

	msg := ValidationErrorMessage(err)
	if msg == "" {
		t.Fatal("expected error message")
	}

	if len(msg) > 0 {
		t.Logf("Error message: %s", msg)
	}
}
