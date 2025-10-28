package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	api "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	persistence_postgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading Submodel Repository Service...")
	log.Default().Println("Config Path:", configPath)
	// Load configuration
	config, err := common.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
		return err
	}

	common.PrintConfiguration(config)

	// Create Chi router
	r := chi.NewRouter()

	// Enable CORS
	common.AddCors(r, config)

	// Add health endpoint
	common.AddHealthEndpoint(r, config)

	// Instantiate generated services & controllers
	// ==== Submodel Repository Service ====
	smDatabase, err := persistence_postgresql.NewPostgreSQLSubmodelBackend("postgres://"+config.Postgres.User+":"+config.Postgres.Password+"@"+config.Postgres.Host+":"+strconv.Itoa(config.Postgres.Port)+"/"+config.Postgres.DBName+"?sslmode=disable", config.Postgres.MaxOpenConnections, config.Postgres.MaxIdleConnections, config.Postgres.ConnMaxLifetimeMinutes, config.Server.CacheEnabled, databaseSchema)
	if err != nil {
		log.Fatalf("Failed to initialize database connection: %v", err)
		return err
	}

	//TEST
	var acc int64
	for i := 0; i < 1000; i++ {
		start := time.Now().Local().UnixMilli()
		persistence_utils.GetSubmodelById(smDatabase.GetDB(), fmt.Sprintf("5_%d", i))
		end := time.Now().Local().UnixMilli()
		fmt.Printf("Total time: %d milliseconds\n", end-start)
		acc += int64(end - start)
	}
	fmt.Printf("Average time: %d milliseconds\n", acc/1000)
	fmt.Println("Total accumulated time:", acc)

	// Same as above but Parallel
	var wg sync.WaitGroup
	threadCount := 100
	iterations := 1000
	perThread := iterations / threadCount

	wg.Add(threadCount)
	startTime := time.Now().UnixMilli()
	for t := 0; t < threadCount; t++ {
		go func(threadID int) {
			defer wg.Done()
			localAcc := int64(0)
			startIdx := threadID * perThread
			endIdx := startIdx + perThread

			for i := startIdx; i < endIdx; i++ {
				start := time.Now().UnixMilli()
				persistence_utils.GetSubmodelById(smDatabase.GetDB(), fmt.Sprintf("5_%d", i))
				end := time.Now().UnixMilli()
				duration := end - start
				fmt.Printf("[Thread %02d] Total time for 5_%d: %d ms\n", threadID, i, duration)
				localAcc += duration
			}

		}(t)
	}

	wg.Wait()
	endTime := time.Now().UnixMilli()
	totalDuration := endTime - startTime
	averageDuration := totalDuration / int64(iterations)
	fmt.Printf("Parallel Execution - Total time: %d ms, Average time per request: %d ms\n", totalDuration, averageDuration)
	// Requests per second
	requestsPerSecond := float64(iterations) / (float64(totalDuration) / 1000.0)
	fmt.Printf("Requests per second: %.2f\n", requestsPerSecond)

	// sm, err := smDatabase.GetSubmodelById("5_1")
	// jsonSubmodel, _ := json.Marshal(sm)
	// fmt.Println(string(jsonSubmodel))
	//TEST

	smSvc := api.NewSubmodelRepositoryAPIAPIService(*smDatabase)
	smCtrl := openapi.NewSubmodelRepositoryAPIAPIController(smSvc, config.Server.ContextPath)
	for _, rt := range smCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// ==== Description Service ====
	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)
	for _, rt := range descCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Start the server
	addr := "0.0.0.0:" + fmt.Sprintf("%d", config.Server.Port)
	log.Printf("▶️  Submodel Repository listening on %s\n", addr)
	// Start server in a goroutine
	go func() {
		if err := http.ListenAndServe(addr, r); err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server...")
	return nil
}

func main() {
	ctx := context.Background()
	//load config path from flag
	configPath := ""
	databaseSchema := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema")
	flag.Parse()

	if databaseSchema != "" {
		_, fileError := os.ReadFile(databaseSchema)
		if fileError != nil {
			fmt.Println("The specified database schema path is invalid or the file was not found.")
			os.Exit(1)
		}
	}

	if err := runServer(ctx, configPath, databaseSchema); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
