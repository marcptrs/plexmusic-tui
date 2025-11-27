package domain

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validator wraps the validator instance with custom validators for Plex domain
type Validator struct {
	v *validator.Validate
}

// NewValidator creates a new validator with custom Plex-specific rules
func NewValidator() *Validator {
	v := validator.New()

	// Register custom validators
	v.RegisterValidation("plextoken", isValidPlexToken)
	v.RegisterValidation("plexurl", isValidPlexURL)
	v.RegisterValidation("plexhost", isValidPlexHost)
	v.RegisterValidation("plexusername", isValidPlexUsername)

	return &Validator{v: v}
}

// ValidateStruct validates a struct with registered rules
func (v *Validator) ValidateStruct(s interface{}) error {
	return v.v.Struct(s)
}

// ValidateVar validates a single variable against a tag
func (v *Validator) ValidateVar(field interface{}, tag string) error {
	return v.v.Var(field, tag)
}

// CredentialsInput represents login form input with validation rules
type CredentialsInput struct {
	Username string `validate:"required,plexusername,min=1,max=100"`
	Password string `validate:"required,min=1,max=500"`
}

// ServerInput represents server selection input with validation rules
type ServerInput struct {
	Host         string `validate:"required,plexhost,min=1,max=255"`
	Port         string `validate:"required,numeric,min=1,max=5"`
	AccessToken  string `validate:"required,plextoken"`
	LocalAddress string `validate:"omitempty,plexhost"`
}

// Validate validates the ServerInput
func (s *ServerInput) Validate() error {
	v := NewValidator()
	return v.ValidateStruct(s)
}

// Validate validates the CredentialsInput
func (c *CredentialsInput) Validate() error {
	v := NewValidator()
	return v.ValidateStruct(c)
}

// isValidPlexToken checks if a token looks like a valid Plex auth token
// Plex tokens are typically alphanumeric strings, 20-100 characters
func isValidPlexToken(fl validator.FieldLevel) bool {
	token := fl.Field().String()

	// Token must not be empty
	if token == "" {
		return false
	}

	// Token should be reasonably sized (Plex tokens are typically 20-100 chars)
	if len(token) < 10 || len(token) > 500 {
		return false
	}

	// Token should contain only alphanumeric and common safe characters
	for _, r := range token {
		isValid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_'
		if !isValid {
			return false
		}
	}

	return true
}

// isValidPlexURL checks if a string is a valid Plex server URL format
func isValidPlexURL(fl validator.FieldLevel) bool {
	urlStr := fl.Field().String()

	if urlStr == "" {
		return false
	}

	// Try to parse as URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Must have a scheme (http or https)
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	// Must have a host
	if u.Host == "" {
		return false
	}

	return true
}

// isValidPlexHost checks if a string is a valid hostname or IP address
// Valid formats: localhost, 127.0.0.1, example.com, 192.168.1.1, example.com:32400
func isValidPlexHost(fl validator.FieldLevel) bool {
	host := fl.Field().String()

	if host == "" {
		return false
	}

	// Remove port if present
	if strings.Contains(host, ":") {
		parts := strings.Split(host, ":")
		if len(parts) != 2 {
			return false
		}
		host = parts[0]
	}

	// Localhost is always valid
	if host == "localhost" {
		return true
	}

	// Try parsing as IP
	u, err := url.Parse("http://" + host)
	if err != nil {
		return false
	}

	// Check if it looks like a valid hostname or IP
	return u.Hostname() != ""
}

// isValidPlexUsername checks if a username meets requirements for Plex authentication
func isValidPlexUsername(fl validator.FieldLevel) bool {
	username := fl.Field().String()

	if username == "" {
		return false
	}

	// Username must be at least 1 char, max 100
	if len(username) > 100 {
		return false
	}

	// Username can be email or username - basic validation
	// Allow letters, numbers, dots, dashes, underscores, and @ for emails
	for _, r := range username {
		isValid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '-' || r == '_' || r == '@'
		if !isValid {
			return false
		}
	}

	return true
}

// ValidationErrorMessage formats validator errors into user-friendly messages
func ValidationErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		return "Validation error: " + err.Error()
	}

	if len(ve) == 0 {
		return ""
	}

	// Build user-friendly error messages
	var messages []string
	for _, fieldError := range ve {
		msg := buildFieldErrorMessage(fieldError)
		messages = append(messages, msg)
	}

	return strings.Join(messages, "\n")
}

// buildFieldErrorMessage creates a user-friendly error message for a single field error
func buildFieldErrorMessage(fe validator.FieldError) string {
	field := fe.Field()
	tag := fe.Tag()
	value := fe.Value()

	switch tag {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "plextoken":
		return fmt.Sprintf("%s must be a valid Plex authentication token (10-500 characters)", field)
	case "plexurl":
		return fmt.Sprintf("%s must be a valid URL (e.g., http://example.com:32400)", field)
	case "plexhost":
		return fmt.Sprintf("%s must be a valid hostname or IP address", field)
	case "plexusername":
		return fmt.Sprintf("%s must be a valid email or username", field)
	case "min":
		return fmt.Sprintf("%s is too short (minimum %s characters)", field, fe.Param())
	case "max":
		return fmt.Sprintf("%s is too long (maximum %s characters)", field, fe.Param())
	case "numeric":
		return fmt.Sprintf("%s must be a number", field)
	default:
		return fmt.Sprintf("%s is invalid (failed validation: %s, value: %v)", field, tag, value)
	}
}
