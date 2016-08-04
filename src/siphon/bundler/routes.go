package bundler

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/nytimes/gziphandler"
)

func initRouter() *mux.Router {
	router := mux.NewRouter()
	router.Handle("/v1/push/{app_id}/",
		gziphandler.GzipHandler(AuthMiddleware(Push))).Methods("GET", "POST")
	router.Handle("/v1/pull/{app_id}/",
		gziphandler.GzipHandler(AuthMiddleware(Pull))).Methods("POST")
	router.Handle("/v1/submit/{app_id}/",
		gziphandler.GzipHandler(AuthMiddleware(Submit))).Methods("POST")
	router.Handle("/v1/healthcheck/",
		gziphandler.GzipHandler(Healthcheck())).Methods("GET")

	return router
}

// Start is the entry point for a bundler server
func Start() {
	log.Print("Creating tables...")
	CreateTables()
	log.Print("Creating buckets...")
	CreateBuckets()
	router := initRouter()

	if os.Getenv("SIPHON_ENV") == "testing" {
		log.Print("Listening on 8000... (test mode)")
		log.Fatal(http.ListenAndServe(":8000", router))
	} else {
		log.Print("Listening on 443...")
		certFile := "/code/.keys/getsiphon-com-bundle.crt"
		keyFile := "/code/.keys/host.pem"
		log.Fatal(http.ListenAndServeTLS(":443", certFile, keyFile, router))
	}
}
