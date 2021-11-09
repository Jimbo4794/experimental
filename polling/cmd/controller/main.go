/*
Copyright 2021 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/tektoncd/experimental/polling/pkg/reconciler/scmpoll"
	// "github.com/tektoncd/experimental/polling/pkg/reconciler/scmpollstate"
	corev1 "k8s.io/api/core/v1"

	"knative.dev/pkg/injection"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/signals"
)

const (
	// ControllerLogKey is the name of the logger for the controller cmd
	ControllerLogKey = "tekton-polling-controller"
)

var (
	namespace = flag.String("namespace", corev1.NamespaceAll, "Namespace to restrict informer to. Optional, defaults to all namespaces.")
)

func main() {
	cfg := injection.ParseAndGetRESTConfigOrDie()
	ctx := injection.WithNamespaceScope(signals.NewContext(), *namespace)

	// Set up liveness and readiness probes.
	mux := http.NewServeMux()

	mux.HandleFunc("/", handler)
	mux.HandleFunc("/health", handler)
	mux.HandleFunc("/readiness", handler)

	port := os.Getenv("PROBES_PORT")
	if port == "" {
		port = "8080"
	}
	go func() {
		log.Printf("Readiness and health check server listening on port %s", port)
		log.Fatal(http.ListenAndServe(":"+port, mux))
	}()

	sharedmain.MainWithConfig(ctx, ControllerLogKey, cfg,
		scmpoll.NewController(*namespace))
	// scmpollstate.NewController(*namespace))
}

func handler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
