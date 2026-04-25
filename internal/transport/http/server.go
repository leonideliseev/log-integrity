// Package httptransport exposes the HTTP server and route registration.
package httptransport

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	"github.com/lenchik/logmonitor/internal/runtimeinfo"
	checkservice "github.com/lenchik/logmonitor/internal/service/check"
	entryservice "github.com/lenchik/logmonitor/internal/service/entry"
	logfileservice "github.com/lenchik/logmonitor/internal/service/logfile"
	serverservice "github.com/lenchik/logmonitor/internal/service/server"
	"github.com/lenchik/logmonitor/internal/transport/http/handlers"
	"github.com/lenchik/logmonitor/internal/transport/http/middleware"
)

// ReadinessFunc computes the current readiness status for the process.
type ReadinessFunc func(ctx context.Context) runtimeinfo.Readiness

// Server wraps the Gin engine and HTTP server for API transport.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// NewServer wires application services into the Gin HTTP layer.
func NewServer(addr string, logger *slog.Logger, authToken string, serverService *serverservice.Service, logFileService *logfileservice.Service, entryService *entryservice.Service, checkService *checkservice.Service, jobs *jobqueue.Manager, runtimeState *runtimeinfo.State, readiness ReadinessFunc) *Server {
	serverHandler := handlers.NewServerHandler(serverService, jobs)
	logFileHandler := handlers.NewLogFileHandler(logFileService, jobs)
	entryHandler := handlers.NewEntryHandler(entryService)
	checkHandler := handlers.NewCheckHandler(checkService, jobs)
	jobHandler := handlers.NewJobHandler(jobs)
	runtimeHandler := handlers.NewRuntimeHandler(runtimeState)
	probeHandler := handlers.NewProbeHandler(func(c *gin.Context) runtimeinfo.Readiness {
		return readiness(c.Request.Context())
	})

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestLogger(logger))
	registerSwagger(engine)

	engine.GET("/healthz", probeHandler.Health)
	engine.GET("/readyz", probeHandler.Ready)

	apiGroup := engine.Group("/api", middleware.APIKeyAuth(authToken))
	{
		apiGroup.GET("/dashboard", serverHandler.Dashboard)
		apiGroup.GET("/problems", serverHandler.ListProblems)
		apiGroup.GET("/runtime/validation", runtimeHandler.Validation)
		apiGroup.GET("/jobs", jobHandler.List)
		apiGroup.GET("/jobs/:id", jobHandler.Get)
		apiGroup.GET("/servers", serverHandler.List)
		apiGroup.GET("/servers/:id", serverHandler.Get)
		apiGroup.POST("/servers", serverHandler.Create)
		apiGroup.POST("/servers/:id/retry", serverHandler.Retry)
		apiGroup.PUT("/servers/:id", serverHandler.Update)
		apiGroup.DELETE("/servers/:id", serverHandler.Delete)
		apiGroup.POST("/servers/discover", serverHandler.Discover)

		apiGroup.GET("/logfiles", logFileHandler.List)
		apiGroup.POST("/logfiles/collect", logFileHandler.Collect)

		apiGroup.GET("/entries", entryHandler.List)

		apiGroup.GET("/checks", checkHandler.List)
		apiGroup.POST("/checks/run", checkHandler.Run)
	}

	return &Server{
		logger: logger,
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           engine,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

// Run starts the API server and gracefully shuts it down when context is canceled.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("http server started", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("api: listen and serve: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
