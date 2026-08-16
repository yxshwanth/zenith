package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/zenith/zenith/internal/service"
	"google.golang.org/grpc/codes"
)

// Gateway provides REST/JSON HTTP endpoints that wrap the gRPC service
type Gateway struct {
	service *service.Service
	router  *http.ServeMux
	server  *http.Server
}

// NewGateway creates a new HTTP gateway
func NewGateway(svc *service.Service) *Gateway {
	g := &Gateway{
		service: svc,
		router:  http.NewServeMux(),
	}
	g.setupRoutes()
	return g
}

// setupRoutes configures all HTTP routes
func (g *Gateway) setupRoutes() {
	// API routes
	g.router.HandleFunc("/api/tuples", g.handleTuples)
	g.router.HandleFunc("/api/check", g.handleCheck)
	g.router.HandleFunc("/api/batch-check", g.handleBatchCheck)
	g.router.HandleFunc("/api/subjects/", g.handleListSubjects)
	g.router.HandleFunc("/api/health", g.handleHealth)
}

// Start starts the HTTP server
func (g *Gateway) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	
	// Apply middleware chain
	handler := corsMiddleware(loggingMiddleware(errorRecoveryMiddleware(g.router)))
	
	g.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Starting HTTP gateway on port %d...", port)
	if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP gateway: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server
func (g *Gateway) Shutdown(ctx context.Context) error {
	if g.server != nil {
		return g.server.Shutdown(ctx)
	}
	return nil
}

// Helper functions for JSON encoding/decoding

func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, statusCode int, errorMsg, detail string) {
	resp := ErrorResponse{
		Error:   errorMsg,
		Message: detail,
		Code:    statusCode,
	}
	writeJSON(w, statusCode, resp)
}

func readJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// Convert gRPC status to HTTP status code
func grpcToHTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DataLoss:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// Helper to parse zookie string to int64
func parseZookie(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// Helper to format zookie int64 to string
func formatZookie(z int64) string {
	return strconv.FormatInt(z, 10)
}

