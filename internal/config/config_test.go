package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Set required environment variables
	os.Setenv("INSTANCE_DOMAIN", "https://test.jamfcloud.com")
	os.Setenv("CLIENT_ID", "test-client")
	os.Setenv("CLIENT_SECRET", "test-secret")

	defer func() {
		os.Unsetenv("INSTANCE_DOMAIN")
		os.Unsetenv("CLIENT_ID")
		os.Unsetenv("CLIENT_SECRET")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.InstanceDomain != "https://test.jamfcloud.com" {
		t.Errorf("Expected InstanceDomain to be 'https://test.jamfcloud.com', got '%s'", cfg.InstanceDomain)
	}

	if cfg.ClientID != "test-client" {
		t.Errorf("Expected ClientID to be 'test-client', got '%s'", cfg.ClientID)
	}

	if cfg.ClientSecret != "test-secret" {
		t.Errorf("Expected ClientSecret to be 'test-secret', got '%s'", cfg.ClientSecret)
	}

	if cfg.AuthMethod != "oauth2" {
		t.Errorf("Expected AuthMethod to be 'oauth2', got '%s'", cfg.AuthMethod)
	}

	if cfg.LocalFolder != "/packages" {
		t.Errorf("Expected LocalFolder to be '/packages', got '%s'", cfg.LocalFolder)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	// Clear environment
	os.Unsetenv("INSTANCE_DOMAIN")
	os.Unsetenv("CLIENT_ID")
	os.Unsetenv("CLIENT_SECRET")

	_, err := Load()
	if err == nil {
		t.Error("Expected Load() to fail with missing required variables")
	}
}

func TestLoadWithCustomAuthMethod(t *testing.T) {
	// Set required environment variables
	os.Setenv("INSTANCE_DOMAIN", "https://test.jamfcloud.com")
	os.Setenv("CLIENT_ID", "test-client")
	os.Setenv("CLIENT_SECRET", "test-secret")
	os.Setenv("AUTH_METHOD", "basic")

	defer func() {
		os.Unsetenv("INSTANCE_DOMAIN")
		os.Unsetenv("CLIENT_ID")
		os.Unsetenv("CLIENT_SECRET")
		os.Unsetenv("AUTH_METHOD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.AuthMethod != "basic" {
		t.Errorf("Expected AuthMethod to be 'basic', got '%s'", cfg.AuthMethod)
	}
}
