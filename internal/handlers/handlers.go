package handlers

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArminDashti/mark-api/internal/auth"
	"github.com/ArminDashti/mark-api/internal/config"
	"github.com/ArminDashti/mark-api/internal/marks"
	"github.com/ArminDashti/mark-api/internal/users"
	"github.com/gin-gonic/gin"
)

// Handler holds shared dependencies.
type Handler struct {
	cfg   config.Config
	users *users.Store
	marks *marks.Service
}

// New creates a Handler.
func New(cfg config.Config, users *users.Store, marksSvc *marks.Service) *Handler {
	return &Handler{cfg: cfg, users: users, marks: marksSvc}
}

func writeError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

func markHTTPError(c *gin.Context, err error) {
	switch {
	case marks.IsNotFound(err):
		writeError(c, http.StatusNotFound, "not found")
	case marks.IsConflict(err):
		writeError(c, http.StatusConflict, "a mark with this kind and slug already exists")
	case marks.IsBadRequest(err):
		writeError(c, http.StatusBadRequest, err.Error())
	default:
		writeError(c, http.StatusInternalServerError, "internal error")
	}
}

// Health returns a simple health payload.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password" binding:"required"`
}

// Login authenticates by username and returns a JWT.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid request")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(c, http.StatusBadRequest, "username is required")
		return
	}

	user, err := h.users.GetByUsername(c.Request.Context(), username)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "could not load user")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(c, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, err := auth.IssueToken(h.cfg.JWTSecret, user.ID, user.Username)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "could not issue token")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":         user.ID,
			"email":      user.Email,
			"username":   user.Username,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
	})
}

// ListMarks returns marks, optionally filtered by kind and search query q (name or slug).
func (h *Handler) ListMarks(c *gin.Context) {
	out, err := h.marks.List(c.Request.Context(), c.Query("kind"), c.Query("q"))
	if err != nil {
		markHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// CreateMark uploads a new logo or icon.
func (h *Handler) CreateMark(c *gin.Context) {
	file, _, err := readOptionalFile(c, "file")
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if file == nil {
		writeError(c, http.StatusBadRequest, "file is required")
		return
	}
	created, err := h.marks.Create(c.Request.Context(), marks.Input{
		Kind: c.PostForm("kind"),
		Slug: c.PostForm("slug"),
		Name: c.PostForm("name"),
	}, *file)
	if err != nil {
		markHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

type updateJSON struct {
	Kind string `json:"kind"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// UpdateMark updates metadata and optionally replaces the file.
func (h *Handler) UpdateMark(c *gin.Context) {
	in := marks.Input{
		Kind: firstNonEmpty(c.PostForm("kind"), c.Query("kind")),
		Slug: firstNonEmpty(c.PostForm("slug"), c.Query("slug")),
		Name: firstNonEmpty(c.PostForm("name"), c.Query("name")),
	}
	if strings.Contains(c.ContentType(), "application/json") {
		var req updateJSON
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid request")
			return
		}
		in = marks.Input{Kind: req.Kind, Slug: req.Slug, Name: req.Name}
	}

	file, _, err := readOptionalFile(c, "file")
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.marks.Update(c.Request.Context(), c.Param("id"), in, file)
	if err != nil {
		markHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DeleteMark removes a mark.
func (h *Handler) DeleteMark(c *gin.Context) {
	if err := h.marks.Delete(c.Request.Context(), c.Param("id")); err != nil {
		markHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ServeMark returns a transparent PNG at the requested size.
func (h *Handler) ServeMark(c *gin.Context) {
	size := 128
	if raw := strings.TrimSpace(c.Query("size")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "size must be an integer")
			return
		}
		size = n
	}
	png, err := h.marks.ServePNG(c.Request.Context(), c.Param("kind"), c.Param("slug"), size)
	if err != nil {
		markHTTPError(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, "image/png", png)
}

func readOptionalFile(c *gin.Context, field string) (*marks.File, string, error) {
	if !strings.Contains(c.ContentType(), "multipart/form-data") {
		return nil, "", nil
	}
	fh, err := c.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, "", nil
		}
		// Gin wraps missing file as a generic error on non-multipart.
		if strings.Contains(strings.ToLower(err.Error()), "no such file") ||
			strings.Contains(strings.ToLower(err.Error()), "http: no such file") {
			return nil, "", nil
		}
		return nil, "", err
	}
	f, err := fh.Open()
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > 8<<20 {
		return nil, "", errors.New("file is too large")
	}
	mime := fh.Header.Get("Content-Type")
	return &marks.File{Bytes: body, MIME: mime, Name: fh.Filename}, fh.Filename, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
