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

	"gorm.io/gorm"
)

func newRunAccessContext(t *testing.T, authEnabled bool) *application_context.MahresourcesContext {
	t.Helper()
	ctx, _ := newRunAccessContextWithDB(t, authEnabled)
	return ctx
}

func newRunAccessContextWithDB(t *testing.T, authEnabled bool) (*application_context.MahresourcesContext, *gorm.DB) {
	t.Helper()
	ctx, db, _ := application_context.CreateContextWithConfig(&application_context.MahresourcesInputConfig{
		MemoryDB: true, MemoryFS: true, FileSavePath: t.TempDir(),
	})
	t.Cleanup(func() { _ = ctx.ReleaseEphemeralDatabase() })
	if err := db.AutoMigrate(&models.Group{}, &models.User{}, &models.ApiToken{}, &models.Session{}); err != nil {
		t.Fatal(err)
	}
	ctx.Config.AuthEnabled = authEnabled
	return ctx, db
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

// A lookup that is cancelled while it runs is cancellation, not revocation.
// The interleave is injected with a query callback rather than raced.
func TestTemplateRunAccessFactoryCancelledDuringLookup(t *testing.T) {
	appCtx, db := newRunAccessContextWithDB(t, true)
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

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	armed := true
	if err := db.Callback().Query().Before("gorm:query").Register("test:cancel_mid_lookup", func(tx *gorm.DB) {
		if armed {
			armed = false
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:cancel_mid_lookup") })

	_, err = check(runCtx)
	if armed {
		t.Fatal("the credential lookup ran no query, so the interleave was not exercised")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled during lookup: %v, want context.Canceled", err)
	}
	if errors.Is(err, application_context.ErrTemplateRunAccessChanged) {
		t.Errorf("cancellation was reported as an access change: %v", err)
	}
}
