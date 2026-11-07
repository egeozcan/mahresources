package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
)

func newRunAccessContext(t *testing.T, authEnabled bool) *application_context.MahresourcesContext {
	t.Helper()
	ctx, db, _ := application_context.CreateContextWithConfig(&application_context.MahresourcesInputConfig{
		MemoryDB: true, MemoryFS: true, FileSavePath: t.TempDir(),
	})
	t.Cleanup(func() { _ = ctx.ReleaseEphemeralDatabase() })
	if err := db.AutoMigrate(&models.Group{}, &models.User{}, &models.ApiToken{}, &models.Session{}); err != nil {
		t.Fatal(err)
	}
	ctx.Config.AuthEnabled = authEnabled
	return ctx
}

func TestTemplateRunAccessFactory(t *testing.T) {
	t.Run("auth disabled keeps the request's principal", func(t *testing.T) {
		appCtx := newRunAccessContext(t, false)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.SystemPrincipal()))
		bound, err := templateRunAccessFactory(appCtx)(req)(context.Background())
		if err != nil || bound == nil {
			t.Fatalf("auth off: %v, %v", bound, err)
		}
	})

	t.Run("auth enabled", func(t *testing.T) {
		appCtx := newRunAccessContext(t, true)
		editor, err := appCtx.CreateUser(&application_context.UserInput{Username: "ed", Password: "password1", Role: models.RoleEditor})
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := appCtx.CreateApiToken(editor.ID, "t", nil)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.FromUser(editor)))
		check := templateRunAccessFactory(appCtx)(req)

		if bound, err := check(context.Background()); err != nil || bound == nil {
			t.Fatalf("unchanged access: %v, %v", bound, err)
		}

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := check(cancelled); !errors.Is(err, context.Canceled) {
			t.Errorf("cancelled context: %v, want context.Canceled", err)
		}

		if _, err := appCtx.UpdateUser(editor.ID, &application_context.UserUpdate{Role: application_context.UserField[models.Role]{Set: true, Value: models.RoleUser}}); err != nil {
			t.Fatal(err)
		}
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := check(ctx); !errors.Is(err, application_context.ErrTemplateRunAccessChanged) {
			t.Errorf("changed role: %v, want ErrTemplateRunAccessChanged", err)
		}
	})
}
