package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/attachments"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/http/response"
	"task-bug-tracker/backend/internal/storage"
)

type AttachmentHandler struct {
	pool    *pgxpool.Pool
	storage storage.Storage
}

func NewAttachmentHandler(pool *pgxpool.Pool, store storage.Storage) *AttachmentHandler {
	if store == nil {
		uploadDir := os.Getenv("UPLOAD_DIR")
		if uploadDir == "" {
			uploadDir = "./uploads"
		}
		var err error
		store, err = storage.NewLocalStorage(uploadDir)
		if err != nil {
			panic(fmt.Sprintf("failed to initialize storage: %v", err))
		}
	}
	return &AttachmentHandler{pool: pool, storage: store}
}

type AttachmentResponse struct {
	ID               uuid.UUID `json:"id"`
	WorkspaceID      uuid.UUID `json:"workspace_id"`
	ItemID           uuid.UUID `json:"item_id"`
	UploaderID       uuid.UUID `json:"uploader_id"`
	OriginalFilename string    `json:"original_filename"`
	MIMEType         string    `json:"mime_type"`
	ByteSize         int64     `json:"byte_size"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	CreatedAt        time.Time `json:"created_at"`
}

func (h *AttachmentHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	itemStr := chi.URLParam(r, "id")

	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Workspace ID tidak valid")
		return
	}
	itemID, err := uuid.Parse(itemStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ITEM_ID", "Item ID tidak valid")
		return
	}

	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		response.Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, attachments.MaxFileBytes+1024*1024)
	if err := r.ParseMultipartForm(attachments.MaxFileBytes + 1024*1024); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "Ukuran file melebihi batas 5 MiB")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "MISSING_FILE", "File gambar 'image' wajib diunggah")
		return
	}
	defer file.Close()

	processed, err := attachments.ProcessAndNormalizeImage(file, header.Filename)
	if err != nil {
		if errors.Is(err, attachments.ErrFileTooLarge) {
			response.Error(w, r, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", err.Error())
		} else if errors.Is(err, attachments.ErrUnsupportedMediaType) {
			response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", err.Error())
		} else {
			response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_IMAGE", err.Error())
		}
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var dummyID uuid.UUID
	err = tx.QueryRow(ctx, "SELECT id FROM work_items WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL FOR UPDATE", itemID, wsID).Scan(&dummyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan")
			return
		}
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal memeriksa item")
		return
	}

	var count int
	err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM item_attachments WHERE item_id = $1 AND deleted_at IS NULL", itemID).Scan(&count)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal menghitung lampiran")
		return
	}

	if count >= 5 {
		response.Error(w, r, http.StatusBadRequest, "ATTACHMENT_LIMIT_EXCEEDED", "Maksimum 5 gambar per item")
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = "." + processed.Format
	}
	storageKey := uuid.New().String() + ext

	if err := h.storage.Save(storageKey, processed.Data); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "STORAGE_ERROR", "Gagal menyimpan file gambar")
		return
	}

	attachID := uuid.New()
	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO item_attachments 
		(id, workspace_id, item_id, uploader_id, original_filename, storage_key, mime_type, byte_size, width, height, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		RETURNING created_at
	`, attachID, wsID, itemID, actor.ID, header.Filename, storageKey, processed.MIMEType, processed.ByteSize, processed.Width, processed.Height).Scan(&createdAt)

	if err != nil {
		_ = h.storage.Delete(storageKey)
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal menyimpan metadata lampiran")
		return
	}

	reqID := middleware.GetRequestID(r.Context())
	detailsJSON := fmt.Sprintf(`{"attachment_id":"%s","filename":"%s"}`, attachID, header.Filename)
	_, _ = tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, workspace_id, entity_type, entity_id, action, details, request_id, created_at)
		VALUES ($1, $2, 'ATTACHMENT', $3, 'UPLOAD_ATTACHMENT', $4::jsonb, $5, NOW())
	`, actor.ID, wsID, attachID, detailsJSON, reqID)

	if err := tx.Commit(ctx); err != nil {
		_ = h.storage.Delete(storageKey)
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal commit transaksi")
		return
	}

	resp := AttachmentResponse{
		ID:               attachID,
		WorkspaceID:      wsID,
		ItemID:           itemID,
		UploaderID:       actor.ID,
		OriginalFilename: header.Filename,
		MIMEType:         processed.MIMEType,
		ByteSize:         processed.ByteSize,
		Width:            processed.Width,
		Height:           processed.Height,
		CreatedAt:        createdAt,
	}
	response.JSON(w, http.StatusCreated, map[string]interface{}{"data": resp})
}

func (h *AttachmentHandler) GetAttachmentContent(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	itemStr := chi.URLParam(r, "id")
	attachStr := chi.URLParam(r, "aid")

	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Workspace ID tidak valid")
		return
	}
	itemID, err := uuid.Parse(itemStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ITEM_ID", "Item ID tidak valid")
		return
	}
	attachID, err := uuid.Parse(attachStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ATTACHMENT_ID", "Attachment ID tidak valid")
		return
	}

	ctx := r.Context()
	var storageKey, mimeType string
	err = h.pool.QueryRow(ctx, `
		SELECT a.storage_key, a.mime_type 
		FROM item_attachments a
		JOIN work_items i ON i.id = a.item_id
		WHERE a.id = $1 AND a.item_id = $2 AND i.workspace_id = $3 AND a.deleted_at IS NULL AND i.deleted_at IS NULL
	`, attachID, itemID, wsID).Scan(&storageKey, &mimeType)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Gambar tidak ditemukan")
			return
		}
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal membaca data gambar")
		return
	}

	file, err := h.storage.Open(storageKey)
	if err != nil {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "File gambar tidak ditemukan pada penyimpan")
		return
	}
	defer file.Close()

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, file)
}

func (h *AttachmentHandler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	itemStr := chi.URLParam(r, "id")
	attachStr := chi.URLParam(r, "aid")

	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Workspace ID tidak valid")
		return
	}
	itemID, err := uuid.Parse(itemStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ITEM_ID", "Item ID tidak valid")
		return
	}
	attachID, err := uuid.Parse(attachStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ATTACHMENT_ID", "Attachment ID tidak valid")
		return
	}

	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		response.Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var storageKey string
	err = tx.QueryRow(ctx, `
		UPDATE item_attachments a
		SET deleted_at = NOW()
		FROM work_items i
		WHERE a.id = $1 AND a.item_id = $2 AND a.item_id = i.id AND i.workspace_id = $3 AND a.deleted_at IS NULL AND i.deleted_at IS NULL
		RETURNING a.storage_key
	`, attachID, itemID, wsID).Scan(&storageKey)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Lampiran tidak ditemukan")
			return
		}
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal menghapus lampiran")
		return
	}

	reqID := middleware.GetRequestID(r.Context())
	detailsJSON := fmt.Sprintf(`{"attachment_id":"%s"}`, attachID)
	_, _ = tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, workspace_id, entity_type, entity_id, action, details, request_id, created_at)
		VALUES ($1, $2, 'ATTACHMENT', $3, 'DELETE_ATTACHMENT', $4::jsonb, $5, NOW())
	`, actor.ID, wsID, attachID, detailsJSON, reqID)

	if err := tx.Commit(ctx); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal commit transaksi")
		return
	}

	_ = h.storage.Delete(storageKey)
	w.WriteHeader(http.StatusNoContent)
}
