package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	api "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	persistence_postgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

func main() {
	// Create Chi router
	r := chi.NewRouter()

	// Enable CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})
	r.Use(c.Handler)

	// Add health endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Instantiate generated services & controllers
	// ==== Discovery Service ====
	smDatabase, err := persistence_postgresql.NewPostgreSQLSubmodelBackend("postgres://admin:admin123@localhost:5432/basyxTestDatabase?sslmode=disable")
	if err != nil {
		log.Fatalf("Failed to initialize database connection: %v", err)
		return
	}
	smSvc := api.NewSubmodelRepositoryAPIAPIService(*smDatabase)
	smCtrl := openapi.NewSubmodelRepositoryAPIAPIController(smSvc)
	for _, rt := range smCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// ==== Description Service ====
	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)
	for _, rt := range descCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	// Add a demo-insert endpoint to bypass validation for POST
	r.Post("/demo-insert", func(w http.ResponseWriter, r *http.Request) {
		m := openapi.Submodel{
			Id:        "sm-99",
			IdShort:   "Demo",
			ModelType: "Submodel",
			Kind:      "Instance",
		}
		_, err := smDatabase.CreateSubmodel(m)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("inserted sm-99"))
	})

	// Start the server
	addr := "0.0.0.0:5004"
	log.Printf("▶️  Submodel Repository listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
