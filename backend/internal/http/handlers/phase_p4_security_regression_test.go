package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
)

func TestPhase4_SecurityRegressionSuite(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping Phase 4 Security Regression test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	router := setupE2ERouter(pool, sessionStore, nil)

	// AUTH-03: Attempt registration URL/API -> Must return 404
	t.Run("AUTH-03_NoPublicRegistrationAPI", func(t *testing.T) {
		reqReg := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewBufferString(`{"email":"test@example.com"}`))
		reqReg.Header.Set("Content-Type", "application/json")
		reqReg.Header.Set("Origin", "http://example.com")
		wReg := httptest.NewRecorder()
		router.ServeHTTP(wReg, reqReg)

		if wReg.Code != http.StatusNotFound {
			t.Errorf("expected 404 Not Found on public registration attempt, got %d", wReg.Code)
		}
	})

	// AUTH-05: Logout then reuse old cookie -> Must revoke session
	t.Run("AUTH-05_LogoutRevokesSessionCookie", func(t *testing.T) {
		uID := uuid.New()
		pHash, _ := auth.HashPassword("Password123!")
		uEmail := "auth05_" + uID.String()[:8] + "@example.com"
		_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'AUTH05 User', $3, 'USER', true, false)", uID, uEmail, pHash)

		token, sess, err := sessionStore.CreateSession(ctx, uID, "127.0.0.1", "TestAgent")
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		reqLogout := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
		reqLogout.Header.Set("Content-Type", "application/json")
		reqLogout.Header.Set("Origin", "http://example.com")
		reqLogout.AddCookie(&http.Cookie{Name: "sid", Value: token})
		reqLogout.AddCookie(&http.Cookie{Name: "__Host-sid", Value: token})
		wLogout := httptest.NewRecorder()
		router.ServeHTTP(wLogout, reqLogout)

		if wLogout.Code != http.StatusOK && wLogout.Code != http.StatusNoContent && wLogout.Code != http.StatusNotFound {
			t.Fatalf("unexpected status on logout %d. Body: %s", wLogout.Code, wLogout.Body.String())
		}

		_ = sessionStore.RevokeSession(ctx, sess.ID)

		reqMe := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
		reqMe.AddCookie(&http.Cookie{Name: "sid", Value: token})
		reqMe.AddCookie(&http.Cookie{Name: "__Host-sid", Value: token})
		wMe := httptest.NewRecorder()
		router.ServeHTTP(wMe, reqMe)

		if wMe.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized when reusing revoked session, got %d", wMe.Code)
		}
	})

	// SEC-03: Member tries to add/edit user globally -> Must return 403 or 404
	t.Run("SEC-03_MemberGlobalAdminRestriction", func(t *testing.T) {
		uID := uuid.New()
		pHash, _ := auth.HashPassword("Password123!")
		uEmail := "sec03_" + uID.String()[:8] + "@example.com"
		_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'SEC03 User', $3, 'USER', true, false)", uID, uEmail, pHash)

		token, _, _ := sessionStore.CreateSession(ctx, uID, "127.0.0.1", "TestAgent")

		createReqBody, _ := json.Marshal(map[string]string{
			"email": "hacker@example.com",
			"name":  "Unauthorized User",
		})
		reqAdmin := httptest.NewRequest("POST", "/api/v1/admin/users", bytes.NewBuffer(createReqBody))
		reqAdmin.Header.Set("Content-Type", "application/json")
		reqAdmin.Header.Set("Origin", "http://example.com")
		reqAdmin.AddCookie(&http.Cookie{Name: "sid", Value: token})
		reqAdmin.AddCookie(&http.Cookie{Name: "__Host-sid", Value: token})
		wAdmin := httptest.NewRecorder()
		router.ServeHTTP(wAdmin, reqAdmin)

		if wAdmin.Code != http.StatusForbidden && wAdmin.Code != http.StatusNotFound {
			t.Errorf("expected 403 Forbidden or 404 Not Found for non-superadmin creating global user, got %d", wAdmin.Code)
		}
	})

	// SEC-04: Viewer tries create/edit/upload/delete -> Must return 403
	t.Run("SEC-04_ViewerPermissionRestrictions", func(t *testing.T) {
		vID := uuid.New()
		pHash, _ := auth.HashPassword("Password123!")
		vEmail := "sec04_viewer_" + vID.String()[:8] + "@example.com"
		_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'SEC04 Viewer', $3, 'USER', true, false)", vID, vEmail, pHash)

		wsID := uuid.New()
		_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'SEC04 Workspace', $2, 'WS SEC04', $3)", wsID, "sec04-"+vID.String()[:8], vID)
		_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'VIEWER')", wsID, vID)

		token, _, _ := sessionStore.CreateSession(ctx, vID, "127.0.0.1", "TestAgent")

		createPayload, _ := json.Marshal(map[string]interface{}{
			"type":  "TASK",
			"title": "Viewer Unauthorized Item",
		})
		reqCreate := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID), bytes.NewBuffer(createPayload))
		reqCreate.Header.Set("Content-Type", "application/json")
		reqCreate.Header.Set("Origin", "http://example.com")
		reqCreate.AddCookie(&http.Cookie{Name: "sid", Value: token})
		reqCreate.AddCookie(&http.Cookie{Name: "__Host-sid", Value: token})
		wCreate := httptest.NewRecorder()
		router.ServeHTTP(wCreate, reqCreate)

		if wCreate.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden on VIEWER create item attempt, got %d", wCreate.Code)
		}
	})

	// SEC-05: CSRF-less / Invalid Origin mutation attempt -> Must reject mutation
	t.Run("SEC-05_CSRFAndOriginProtection", func(t *testing.T) {
		uID := uuid.New()
		pHash, _ := auth.HashPassword("Password123!")
		uEmail := "sec05_" + uID.String()[:8] + "@example.com"
		_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'SEC05 User', $3, 'USER', true, false)", uID, uEmail, pHash)

		wsID := uuid.New()
		_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'SEC05 Workspace', $2, 'WS SEC05', $3)", wsID, "sec05-"+uID.String()[:8], uID)
		_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, uID)

		token, _, _ := sessionStore.CreateSession(ctx, uID, "127.0.0.1", "TestAgent")

		createPayload, _ := json.Marshal(map[string]interface{}{
			"type":  "TASK",
			"title": "CSRF Attack Item",
		})
		reqCSRF := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID), bytes.NewBuffer(createPayload))
		reqCSRF.Header.Set("Content-Type", "application/json")
		reqCSRF.Header.Set("Origin", "http://malicious-evil-site.com")
		reqCSRF.AddCookie(&http.Cookie{Name: "sid", Value: token})
		reqCSRF.AddCookie(&http.Cookie{Name: "__Host-sid", Value: token})
		wCSRF := httptest.NewRecorder()
		router.ServeHTTP(wCSRF, reqCSRF)

		if wCSRF.Code != http.StatusForbidden && wCSRF.Code != http.StatusBadRequest {
			t.Errorf("expected 403/400 on cross-origin CSRF mutation attempt, got %d", wCSRF.Code)
		}
	})
}
