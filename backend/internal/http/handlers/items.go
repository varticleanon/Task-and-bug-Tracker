package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/http/response"
)

type AttachmentMeta struct {
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

type ItemResponse struct {
	ID               uuid.UUID        `json:"id"`
	WorkspaceID      uuid.UUID        `json:"workspace_id"`
	Type             string           `json:"type"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	Status           string           `json:"status"`
	Priority         string           `json:"priority"`
	AssigneeID       *uuid.UUID       `json:"assignee_id"`
	DueDate          *string          `json:"due_date"`
	StepsToReproduce *string          `json:"steps_to_reproduce"`
	ExpectedResult   *string          `json:"expected_result"`
	ActualResult     *string          `json:"actual_result"`
	CreatedBy        uuid.UUID        `json:"created_by"`
	UpdatedBy        uuid.UUID        `json:"updated_by"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	Version          int              `json:"version"`
	Attachments      []AttachmentMeta `json:"attachments"`
}

type WorkItemResponse = ItemResponse

type ItemHandler struct {
	pool *pgxpool.Pool
}

func NewItemHandler(pool *pgxpool.Pool) *ItemHandler {
	return &ItemHandler{pool: pool}
}

func fetchAttachmentsForItem(ctx context.Context, pool *pgxpool.Pool, wsID, itemID uuid.UUID) ([]AttachmentMeta, error) {
	query := `
		SELECT id, workspace_id, item_id, uploader_id, original_filename, mime_type, byte_size, width, height, created_at
		FROM item_attachments
		WHERE workspace_id = $1 AND item_id = $2 AND deleted_at IS NULL
		ORDER BY created_at ASC
	`
	rows, err := pool.Query(ctx, query, wsID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make([]AttachmentMeta, 0)
	for rows.Next() {
		var att AttachmentMeta
		if err := rows.Scan(
			&att.ID,
			&att.WorkspaceID,
			&att.ItemID,
			&att.UploaderID,
			&att.OriginalFilename,
			&att.MIMEType,
			&att.ByteSize,
			&att.Width,
			&att.Height,
			&att.CreatedAt,
		); err != nil {
			return nil, err
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

func (h *ItemHandler) CreateItem(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Workspace ID tidak valid")
		return
	}

	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		response.Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan")
		return
	}

	var req struct {
		Type             string     `json:"type"`
		Title            string     `json:"title"`
		Description      string     `json:"description"`
		Status           string     `json:"status"`
		Priority         string     `json:"priority"`
		AssigneeID       *uuid.UUID `json:"assignee_id"`
		DueDate          *string    `json:"due_date"`
		StepsToReproduce *string    `json:"steps_to_reproduce"`
		ExpectedResult   *string    `json:"expected_result"`
		ActualResult     *string    `json:"actual_result"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "MALFORMED_JSON", "Payload JSON tidak valid")
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if len(req.Title) < 3 || len(req.Title) > 180 {
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_TITLE", "Title harus 3–180 karakter")
		return
	}

	if req.Type == "" {
		req.Type = "TASK"
	} else if req.Type != "TASK" && req.Type != "BUG" {
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_TYPE", "Tipe harus TASK atau BUG")
		return
	}

	if req.Status == "" {
		req.Status = "TODO"
	}
	validStatuses := map[string]bool{"TODO": true, "IN_PROGRESS": true, "BLOCKED": true, "IN_REVIEW": true, "DONE": true}
	if !validStatuses[req.Status] {
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_STATUS", "Status tidak valid")
		return
	}

	if req.Priority == "" {
		req.Priority = "MEDIUM"
	}
	validPriorities := map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}
	if !validPriorities[req.Priority] {
		response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_PRIORITY", "Priority tidak valid")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	if req.AssigneeID != nil {
		var isEligible bool
		err = tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM workspace_members wm 
				JOIN users u ON u.id = wm.user_id 
				WHERE wm.workspace_id = $1 AND wm.user_id = $2 AND u.is_active = true
			)
		`, wsID, *req.AssigneeID).Scan(&isEligible)

		if err != nil || !isEligible {
			response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_ASSIGNEE", "PIC harus berupa member aktif di workspace ini")
			return
		}
	}

	var dueDateVal *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		tVal, err := time.Parse("2006-01-02", *req.DueDate)
		if err != nil {
			response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_DUE_DATE", "Due date harus format YYYY-MM-DD")
			return
		}
		dueDateVal = &tVal
	}

	itemID := uuid.New()
	var createdAt, updatedAt time.Time
	query := `
		INSERT INTO work_items 
		(id, workspace_id, type, title, description, status, priority, assignee_id, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13, 1, NOW(), NOW())
		RETURNING created_at, updated_at
	`
	err = tx.QueryRow(ctx, query,
		itemID, wsID, req.Type, req.Title, req.Description, req.Status, req.Priority, req.AssigneeID, dueDateVal,
		req.StepsToReproduce, req.ExpectedResult, req.ActualResult, actor.ID,
	).Scan(&createdAt, &updatedAt)

	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal menyimpan work item")
		return
	}

	reqID := middleware.GetRequestID(r.Context())
	detailsJSON := fmt.Sprintf(`{"title":"%s","type":"%s"}`, req.Title, req.Type)
	_, _ = tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, workspace_id, entity_type, entity_id, action, details, request_id, created_at)
		VALUES ($1, $2, 'ITEM', $3, 'CREATE_ITEM', $4::jsonb, $5, NOW())
	`, actor.ID, wsID, itemID, detailsJSON, reqID)

	if err := tx.Commit(ctx); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal commit transaksi")
		return
	}

	var dueDateStr *string
	if dueDateVal != nil {
		formatted := dueDateVal.Format("2006-01-02")
		dueDateStr = &formatted
	}

	resp := ItemResponse{
		ID:               itemID,
		WorkspaceID:      wsID,
		Type:             req.Type,
		Title:            req.Title,
		Description:      req.Description,
		Status:           req.Status,
		Priority:         req.Priority,
		AssigneeID:       req.AssigneeID,
		DueDate:          dueDateStr,
		StepsToReproduce: req.StepsToReproduce,
		ExpectedResult:   req.ExpectedResult,
		ActualResult:     req.ActualResult,
		CreatedBy:        actor.ID,
		UpdatedBy:        actor.ID,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		Version:          1,
		Attachments:      []AttachmentMeta{},
	}
	response.JSON(w, http.StatusCreated, map[string]interface{}{"data": resp})
}

func (h *ItemHandler) GetItem(w http.ResponseWriter, r *http.Request) {
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

	ctx := r.Context()
	var item ItemResponse
	var dueDateVal *time.Time

	query := `
		SELECT id, workspace_id, type, title, description, status, priority, assignee_id, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by, version, created_at, updated_at
		FROM work_items
		WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
	`
	err = h.pool.QueryRow(ctx, query, itemID, wsID).Scan(
		&item.ID, &item.WorkspaceID, &item.Type, &item.Title, &item.Description, &item.Status, &item.Priority,
		&item.AssigneeID, &dueDateVal, &item.StepsToReproduce, &item.ExpectedResult, &item.ActualResult,
		&item.CreatedBy, &item.UpdatedBy, &item.Version, &item.CreatedAt, &item.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan")
			return
		}
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal membaca item")
		return
	}

	if dueDateVal != nil {
		formatted := dueDateVal.Format("2006-01-02")
		item.DueDate = &formatted
	}

	atts, err := fetchAttachmentsForItem(ctx, h.pool, wsID, itemID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal membaca lampiran item")
		return
	}
	item.Attachments = atts

	response.JSON(w, http.StatusOK, map[string]interface{}{"data": item})
}

func (h *ItemHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
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

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "Gagal membaca body request")
		return
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &rawMap); err != nil {
		response.Error(w, r, http.StatusBadRequest, "MALFORMED_JSON", "Payload JSON tidak valid")
		return
	}

	versionRaw, ok := rawMap["version"]
	if !ok {
		response.Error(w, r, http.StatusBadRequest, "MISSING_VERSION", "Field 'version' wajib disertakan")
		return
	}
	var reqVersion int
	if err := json.Unmarshal(versionRaw, &reqVersion); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_VERSION", "Field 'version' tidak valid")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var current ItemResponse
	var dueDateVal *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id, workspace_id, type, title, description, status, priority, assignee_id, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by, version, created_at, updated_at
		FROM work_items
		WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, itemID, wsID).Scan(
		&current.ID, &current.WorkspaceID, &current.Type, &current.Title, &current.Description, &current.Status, &current.Priority,
		&current.AssigneeID, &dueDateVal, &current.StepsToReproduce, &current.ExpectedResult, &current.ActualResult,
		&current.CreatedBy, &current.UpdatedBy, &current.Version, &current.CreatedAt, &current.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan")
			return
		}
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal membaca item")
		return
	}

	if current.Version != reqVersion {
		response.Error(w, r, http.StatusConflict, "VERSION_CONFLICT", "Item telah diperbarui oleh pengguna lain. Muat ulang data.")
		return
	}

	if rawVal, exists := rawMap["type"]; exists {
		var newType string
		_ = json.Unmarshal(rawVal, &newType)
		if newType == "TASK" || newType == "BUG" {
			if current.Type == "BUG" && newType == "TASK" {
				current.StepsToReproduce = nil
				current.ExpectedResult = nil
				current.ActualResult = nil
			}
			current.Type = newType
		}
	}

	if rawVal, exists := rawMap["title"]; exists {
		var newTitle string
		_ = json.Unmarshal(rawVal, &newTitle)
		newTitle = strings.TrimSpace(newTitle)
		if len(newTitle) < 3 || len(newTitle) > 180 {
			response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_TITLE", "Title harus 3–180 karakter")
			return
		}
		current.Title = newTitle
	}

	if rawVal, exists := rawMap["description"]; exists {
		var newDesc string
		_ = json.Unmarshal(rawVal, &newDesc)
		current.Description = newDesc
	}

	if rawVal, exists := rawMap["status"]; exists {
		var newStatus string
		_ = json.Unmarshal(rawVal, &newStatus)
		validStatuses := map[string]bool{"TODO": true, "IN_PROGRESS": true, "BLOCKED": true, "IN_REVIEW": true, "DONE": true}
		if validStatuses[newStatus] {
			current.Status = newStatus
		}
	}

	if rawVal, exists := rawMap["priority"]; exists {
		var newPriority string
		_ = json.Unmarshal(rawVal, &newPriority)
		validPriorities := map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}
		if validPriorities[newPriority] {
			current.Priority = newPriority
		}
	}

	if rawVal, exists := rawMap["assignee_id"]; exists {
		if bytes.Equal(rawVal, []byte("null")) {
			current.AssigneeID = nil
		} else {
			var newAssigneeStr string
			if err := json.Unmarshal(rawVal, &newAssigneeStr); err == nil && newAssigneeStr != "" {
				parsedAssignee, err := uuid.Parse(newAssigneeStr)
				if err != nil {
					response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_ASSIGNEE", "Format Assignee ID tidak valid")
					return
				}
				var isEligible bool
				err = tx.QueryRow(ctx, `
					SELECT EXISTS (
						SELECT 1 FROM workspace_members wm 
						JOIN users u ON u.id = wm.user_id 
						WHERE wm.workspace_id = $1 AND wm.user_id = $2 AND u.is_active = true
					)
				`, wsID, parsedAssignee).Scan(&isEligible)

				if err != nil || !isEligible {
					response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_ASSIGNEE", "PIC harus berupa member aktif di workspace ini")
					return
				}
				current.AssigneeID = &parsedAssignee
			}
		}
	}

	if rawVal, exists := rawMap["due_date"]; exists {
		if bytes.Equal(rawVal, []byte("null")) {
			dueDateVal = nil
		} else {
			var dateStr string
			if err := json.Unmarshal(rawVal, &dateStr); err == nil && dateStr != "" {
				tVal, err := time.Parse("2006-01-02", dateStr)
				if err != nil {
					response.Error(w, r, http.StatusUnprocessableEntity, "INVALID_DUE_DATE", "Due date harus format YYYY-MM-DD")
					return
				}
				dueDateVal = &tVal
			}
		}
	}

	if current.Type == "BUG" {
		if rawVal, exists := rawMap["steps_to_reproduce"]; exists {
			var val string
			_ = json.Unmarshal(rawVal, &val)
			current.StepsToReproduce = &val
		}
		if rawVal, exists := rawMap["expected_result"]; exists {
			var val string
			_ = json.Unmarshal(rawVal, &val)
			current.ExpectedResult = &val
		}
		if rawVal, exists := rawMap["actual_result"]; exists {
			var val string
			_ = json.Unmarshal(rawVal, &val)
			current.ActualResult = &val
		}
	}

	var newVersion int
	var newUpdatedAt time.Time
	query := `
		UPDATE work_items 
		SET type = $1, title = $2, description = $3, status = $4, priority = $5, assignee_id = $6, due_date = $7,
		    steps_to_reproduce = $8, expected_result = $9, actual_result = $10, updated_by = $11, updated_at = NOW(), version = version + 1
		WHERE id = $12 AND workspace_id = $13 AND version = $14 AND deleted_at IS NULL
		RETURNING version, updated_at
	`
	err = tx.QueryRow(ctx, query,
		current.Type, current.Title, current.Description, current.Status, current.Priority, current.AssigneeID, dueDateVal,
		current.StepsToReproduce, current.ExpectedResult, current.ActualResult, actor.ID, itemID, wsID, reqVersion,
	).Scan(&newVersion, &newUpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			response.Error(w, r, http.StatusConflict, "VERSION_CONFLICT", "Item telah diperbarui oleh pengguna lain. Muat ulang data.")
			return
		}
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal memperbarui work item")
		return
	}

	reqID := middleware.GetRequestID(r.Context())
	detailsJSON := fmt.Sprintf(`{"version":%d,"status":"%s"}`, newVersion, current.Status)
	_, _ = tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, workspace_id, entity_type, entity_id, action, details, request_id, created_at)
		VALUES ($1, $2, 'ITEM', $3, 'UPDATE_ITEM', $4::jsonb, $5, NOW())
	`, actor.ID, wsID, itemID, detailsJSON, reqID)

	if err := tx.Commit(ctx); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal commit transaksi")
		return
	}

	current.Version = newVersion
	current.UpdatedAt = newUpdatedAt
	current.UpdatedBy = actor.ID
	if dueDateVal != nil {
		formatted := dueDateVal.Format("2006-01-02")
		current.DueDate = &formatted
	}

	atts, _ := fetchAttachmentsForItem(r.Context(), h.pool, wsID, itemID)
	current.Attachments = atts

	response.JSON(w, http.StatusOK, map[string]interface{}{"data": current})
}

func (h *ItemHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
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

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, "UPDATE work_items SET deleted_at = NOW() WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL", itemID, wsID)
	if err != nil || result.RowsAffected() == 0 {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan")
		return
	}

	reqID := middleware.GetRequestID(r.Context())
	_, _ = tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, workspace_id, entity_type, entity_id, action, details, request_id, created_at)
		VALUES ($1, $2, 'ITEM', $3, 'DELETE_ITEM', '{}'::jsonb, $4, NOW())
	`, actor.ID, wsID, itemID, reqID)

	if err := tx.Commit(ctx); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal commit transaksi")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ItemHandler) SoftDeleteItem(w http.ResponseWriter, r *http.Request) {
	h.DeleteItem(w, r)
}

func (h *ItemHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Workspace ID tidak valid")
		return
	}

	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	whereClauses := []string{"workspace_id = $1", "deleted_at IS NULL"}
	args := []interface{}{wsID}
	argIdx := 2

	if itemType := strings.ToUpper(strings.TrimSpace(q.Get("type"))); itemType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, itemType)
		argIdx++
	}

	if status := strings.ToUpper(strings.TrimSpace(q.Get("status"))); status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	if priority := strings.ToUpper(strings.TrimSpace(q.Get("priority"))); priority != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, priority)
		argIdx++
	}

	if q.Get("unassigned") == "true" {
		whereClauses = append(whereClauses, "assignee_id IS NULL")
	} else if assigneeIDStr := q.Get("assignee_id"); assigneeIDStr != "" {
		if parsedAssignee, err := uuid.Parse(assigneeIDStr); err == nil {
			whereClauses = append(whereClauses, fmt.Sprintf("assignee_id = $%d", argIdx))
			args = append(args, parsedAssignee)
			argIdx++
		}
	}

	if search := strings.TrimSpace(q.Get("search")); search != "" {
		if len(search) > 100 {
			search = search[:100]
		}
		escaped := "%" + escapeLike(search) + "%"
		whereClauses = append(whereClauses, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx+1))
		args = append(args, escaped, escaped)
		argIdx += 2
	}

	if dueBeforeStr := q.Get("due_before"); dueBeforeStr != "" {
		if tVal, err := time.Parse("2006-01-02", dueBeforeStr); err == nil {
			whereClauses = append(whereClauses, fmt.Sprintf("due_date <= $%d", argIdx))
			args = append(args, tVal)
			argIdx++
		}
	}

	if dueAfterStr := q.Get("due_after"); dueAfterStr != "" {
		if tVal, err := time.Parse("2006-01-02", dueAfterStr); err == nil {
			whereClauses = append(whereClauses, fmt.Sprintf("due_date >= $%d", argIdx))
			args = append(args, tVal)
			argIdx++
		}
	}

	whereStmt := strings.Join(whereClauses, " AND ")
	ctx := r.Context()

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM work_items WHERE %s", whereStmt)
	var total int
	err = h.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal menghitung total item")
		return
	}

	query := fmt.Sprintf(`
		SELECT id, workspace_id, type, title, description, status, priority, assignee_id, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by, version, created_at, updated_at
		FROM work_items
		WHERE %s
		ORDER BY updated_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, whereStmt, argIdx, argIdx+1)

	queryArgs := append(args, pageSize, offset)

	rows, err := h.pool.Query(ctx, query, queryArgs...)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal mengambil daftar item")
		return
	}
	defer rows.Close()

	items := make([]ItemResponse, 0)
	for rows.Next() {
		var item ItemResponse
		var dueDateVal *time.Time
		_ = rows.Scan(
			&item.ID, &item.WorkspaceID, &item.Type, &item.Title, &item.Description, &item.Status, &item.Priority,
			&item.AssigneeID, &dueDateVal, &item.StepsToReproduce, &item.ExpectedResult, &item.ActualResult,
			&item.CreatedBy, &item.UpdatedBy, &item.Version, &item.CreatedAt, &item.UpdatedAt,
		)
		if dueDateVal != nil {
			formatted := dueDateVal.Format("2006-01-02")
			item.DueDate = &formatted
		}
		item.Attachments = []AttachmentMeta{}
		items = append(items, item)
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	meta := map[string]interface{}{
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages,
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{"data": items, "meta": meta})
}
