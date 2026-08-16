package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenith/zenith/internal/api"
	"github.com/zenith/zenith/internal/cache"
	"github.com/zenith/zenith/internal/config"
	"github.com/zenith/zenith/internal/db"
	"github.com/zenith/zenith/internal/engine"
	"github.com/zenith/zenith/internal/gateway"
	_ "github.com/zenith/zenith/internal/metrics" // Initialize metrics
	"github.com/zenith/zenith/internal/middleware"
	"github.com/zenith/zenith/internal/observability"
	"github.com/zenith/zenith/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"net/http"
)

var (
	configFile = flag.String("config", "", "Path to configuration file (YAML or JSON)")
	// Flags for overriding config (priority: flags > env > config file > defaults)
	port              = flag.Int("port", -1, "gRPC server port (overrides config, -1 means use config)")
	dbConnStr         = flag.String("db", "", "Database connection string (overrides config)")
	enableReflection  = flag.String("reflection", "", "Enable gRPC reflection (overrides config: true/false)")
	maxDepth          = flag.Int("max-depth", -1, "Maximum recursion depth for userset expansion (overrides config, -1 means use config)")
	checkTimeout      = flag.Int64("check-timeout", -1, "Check operation timeout in milliseconds (overrides config, -1 means use config)")
	enableCache       = flag.String("enable-cache", "", "Enable caching (overrides config: true/false)")
	cacheTTLPositive  = flag.Duration("cache-ttl-positive", 0, "TTL for positive cache entries (overrides config)")
	cacheTTLNegative  = flag.Duration("cache-ttl-negative", 0, "TTL for negative cache entries (overrides config)")
	cacheSize         = flag.Int("cache-size", -1, "Maximum cache entries (overrides config, -1 means use config)")
	enableTracing     = flag.String("enable-tracing", "", "Enable OpenTelemetry tracing (overrides config: true/false)")
	tracingEndpoint   = flag.String("tracing-endpoint", "", "OTel collector endpoint (overrides config)")
	metricsPort       = flag.Int("metrics-port", -1, "HTTP port for Prometheus metrics (overrides config, -1 means use config)")
	httpPort          = flag.Int("http-port", -1, "HTTP gateway port (overrides config, -1 means use config)")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override with flags if provided
	if *port >= 0 {
		cfg.Server.Port = *port
	}
	if *dbConnStr != "" {
		cfg.Database.ConnectionString = *dbConnStr
	}
	if *enableReflection != "" {
		cfg.Server.Reflection = *enableReflection == "true" || *enableReflection == "1"
	}
	if *maxDepth >= 0 {
		cfg.Engine.MaxDepth = *maxDepth
	}
	if *checkTimeout >= 0 {
		cfg.Engine.CheckTimeout = *checkTimeout
	}
	if *enableCache != "" {
		cfg.Cache.Enabled = *enableCache == "true" || *enableCache == "1"
	}
	if *cacheTTLPositive > 0 {
		cfg.Cache.TTLPositive = *cacheTTLPositive
	}
	if *cacheTTLNegative > 0 {
		cfg.Cache.TTLNegative = *cacheTTLNegative
	}
	if *cacheSize >= 0 {
		cfg.Cache.Size = *cacheSize
	}
	if *enableTracing != "" {
		cfg.Tracing.Enabled = *enableTracing == "true" || *enableTracing == "1"
	}
	if *tracingEndpoint != "" {
		cfg.Tracing.Endpoint = *tracingEndpoint
	}
	if *metricsPort >= 0 {
		cfg.Server.MetricsPort = *metricsPort
	}
	if *httpPort >= 0 {
		cfg.Server.HTTPPort = *httpPort
	}

	// Validate final configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Initialize OpenTelemetry tracing (if enabled)
	var shutdownTracing func()
	if cfg.Tracing.Enabled {
		var err error
		shutdownTracing, err = observability.InitTracing("zenith", cfg.Tracing.Endpoint)
		if err != nil {
			log.Printf("Warning: Failed to initialize tracing: %v (continuing without tracing)", err)
		} else {
			log.Printf("Tracing initialized: endpoint=%s", cfg.Tracing.Endpoint)
		}
		if shutdownTracing != nil {
			defer shutdownTracing()
		}
	}

	// Initialize database connection
	log.Println("Connecting to database...")
	database, err := db.NewDBWithConfig(cfg.Database.ConnectionString, &db.PoolConfig{
		MaxOpenConns:     cfg.Database.MaxOpenConns,
		MaxIdleConns:     cfg.Database.MaxIdleConns,
		ConnMaxLifetime:  cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime:  cfg.Database.ConnMaxIdleTime,
		HealthCheckInterval: cfg.Database.HealthCheckInterval,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()
	log.Println("Database connection established")

	// Run migrations
	log.Println("Running migrations...")
	ctx := context.Background()
	// Note: Database should be created manually or via connection string
	// CockroachDB doesn't support CREATE DATABASE IF NOT EXISTS in transactions
	migrationSQL := []string{
		`CREATE TABLE IF NOT EXISTS relation_tuples (
			namespace STRING NOT NULL,
			object_id STRING NOT NULL,
			relation STRING NOT NULL,
			subject_namespace STRING NOT NULL,
			subject_id STRING NOT NULL,
			subject_relation STRING NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (namespace, object_id, relation, subject_namespace, subject_id, subject_relation),
			INDEX idx_object_relation (namespace, object_id, relation),
			INDEX idx_subject (subject_namespace, subject_id, subject_relation)
		);`,
	}
	if err := database.RunMigrations(ctx, migrationSQL); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Migrations completed")

	// Create tuple repository
	repo := db.NewTupleRepo(database)

	// Initialize cache (if enabled)
	var cacheInstance *cache.Cache
	if cfg.Cache.Enabled {
		cacheInstance, err = cache.NewCache(cfg.Cache.Size, cfg.Cache.TTLPositive, cfg.Cache.TTLNegative)
		if err != nil {
			log.Fatalf("Failed to create cache: %v", err)
		}
		log.Printf("Cache initialized: size=%d, ttl-positive=%v, ttl-negative=%v", cfg.Cache.Size, cfg.Cache.TTLPositive, cfg.Cache.TTLNegative)
	}

	// Create expansion engine (with cache if enabled)
	var expander *engine.ExpansionEngine
	if cacheInstance != nil {
		expander = engine.NewExpansionEngineFromRepoWithCache(repo, cacheInstance, cfg.Engine.MaxDepth, cfg.Engine.CheckTimeout)
	} else {
		expander = engine.NewExpansionEngineFromRepo(repo, cfg.Engine.MaxDepth, cfg.Engine.CheckTimeout)
	}
	log.Printf("Expansion engine initialized: max-depth=%d, timeout=%dms", cfg.Engine.MaxDepth, cfg.Engine.CheckTimeout)

	// Create service (with cache if enabled)
	var zenithService *service.Service
	if cacheInstance != nil {
		zenithService = service.NewServiceWithCache(repo, expander, cacheInstance)
	} else {
		zenithService = service.NewService(repo, expander)
	}

	// Setup gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.Port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create gRPC server with OpenTelemetry instrumentation (if enabled)
	var grpcServer *grpc.Server
	var serverOpts []grpc.ServerOption
	
	if cfg.Tracing.Enabled {
		// Use otelgrpc stats handler for automatic tracing
		statsHandler := otelgrpc.NewServerHandler()
		serverOpts = append(serverOpts, grpc.StatsHandler(statsHandler))
	}
	
	// Add rate limiting interceptor if enabled
	if cfg.RateLimit.Enabled {
		rateLimiter := middleware.NewRateLimiter(
			cfg.RateLimit.GlobalRPS,
			cfg.RateLimit.PerClientRPS,
			cfg.RateLimit.BurstSize,
		)
		serverOpts = append(serverOpts, grpc.UnaryInterceptor(rateLimiter.UnaryInterceptor()))
		serverOpts = append(serverOpts, grpc.StreamInterceptor(rateLimiter.StreamInterceptor()))
		log.Printf("Rate limiting enabled: global_rps=%d, per_client_rps=%d, burst=%d",
			cfg.RateLimit.GlobalRPS, cfg.RateLimit.PerClientRPS, cfg.RateLimit.BurstSize)
	}
	
	grpcServer = grpc.NewServer(serverOpts...)

	// Register services
	api.RegisterZenithServer(grpcServer, zenithService)

	// Register health check service
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// Enable reflection for development
	if cfg.Server.Reflection {
		reflection.Register(grpcServer)
		log.Println("gRPC reflection enabled")
	}

	// Start metrics HTTP server
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.MetricsPort),
		Handler: metricsMux,
	}
	go func() {
		log.Printf("Starting metrics server on port %d...", cfg.Server.MetricsPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Failed to start metrics server: %v", err)
		}
	}()

	// Start HTTP gateway server
	httpGateway := gateway.NewGateway(zenithService)
	go func() {
		log.Printf("Starting HTTP gateway on port %d...", cfg.Server.HTTPPort)
		if err := httpGateway.Start(cfg.Server.HTTPPort); err != nil {
			log.Printf("Failed to start HTTP gateway: %v", err)
		}
	}()

	// Start gRPC server in a goroutine
	go func() {
		log.Printf("Starting gRPC server on port %d...", cfg.Server.Port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown metrics server
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down metrics server: %v", err)
	}

	// Shutdown HTTP gateway
	if err := httpGateway.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down HTTP gateway: %v", err)
	}

	// Stop accepting new connections
	grpcServer.GracefulStop()

	// Mark health as not serving
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	select {
	case <-shutdownCtx.Done():
		log.Println("Shutdown timeout exceeded, forcing stop")
		grpcServer.Stop()
	default:
		log.Println("Server stopped gracefully")
	}
}

