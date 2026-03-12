package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestOpenSettingsStartsLauncherBeforeOpeningBrowser(t *testing.T) {
	var calls []string
	app := &App{
		settingsURL: "http://127.0.0.1:18800",
		ensureSettingsServiceFn: func() error {
			calls = append(calls, "ensure")
			return nil
		},
		openBrowserFn: func(url string) {
			calls = append(calls, url)
		},
	}

	app.OpenSettings()

	want := []string{"ensure", "http://127.0.0.1:18800"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("OpenSettings() call order = %#v, want %#v", calls, want)
	}
}

func TestOpenSettingsSkipsBrowserWhenLauncherStartFails(t *testing.T) {
	app := &App{
		settingsURL: "http://127.0.0.1:18800",
		ensureSettingsServiceFn: func() error {
			return errors.New("launcher failed")
		},
		openBrowserFn: func(string) {
			t.Fatal("OpenSettings() should not open the browser when launcher startup fails")
		},
	}

	app.OpenSettings()
}

func TestStartManagedServicesDoesNotStartSettingsLauncher(t *testing.T) {
	var calls []string
	app := &App{
		ensureGatewayServiceFn: func() error {
			calls = append(calls, "gateway")
			return nil
		},
		ensurePlatformServiceFn: func() error {
			calls = append(calls, "platform")
			return nil
		},
		ensureSettingsServiceFn: func() error {
			calls = append(calls, "settings")
			return nil
		},
	}

	app.startManagedServices()

	want := []string{"gateway", "platform"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("startManagedServices() calls = %#v, want %#v", calls, want)
	}
}
