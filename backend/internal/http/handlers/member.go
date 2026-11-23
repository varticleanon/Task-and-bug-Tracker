package handlers

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MemberHandler struct {
	pool *pgxpool.Pool
}

func NewMemberHandler(pool *pgxpool.Pool) *MemberHandler {
	return &MemberHandler{pool: pool}
}

func (h *MemberHandler) ListMembers(w http.ResponseWriter, r *http.Request)      {}
func (h *MemberHandler) AddMember(w http.ResponseWriter, r *http.Request)        {}
func (h *MemberHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {}
func (h *MemberHandler) RemoveMember(w http.ResponseWriter, r *http.Request)     {}
