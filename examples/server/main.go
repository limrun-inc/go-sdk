package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/limrun-inc/go-sdk/api"
)

func main() {
	token := os.Getenv("LIM_TOKEN") // lim_yourtoken
	limrun := api.NewDefaultClient(token)

	s := http.Server{
		Addr: ":8081",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := &api.AndroidInstanceCreate{
				Spec: api.NewOptAndroidInstanceCreateSpec(api.AndroidInstanceCreateSpec{}),
			}
			clientIp := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
			if clientIp != "" {
				log.Printf("Using client IP %s as scheduling clue", clientIp)
				body.Spec.Value.Clues = append(body.Spec.Value.Clues, api.AndroidInstanceCreateSpecCluesItem{
					Kind:     api.AndroidInstanceCreateSpecCluesItemKindClientIP,
					ClientIp: api.NewOptString(clientIp),
				})
			} else {
				log.Println("No client IP specified as scheduling clue")
			}
			instance, err := limrun.CreateAndroidInstance(r.Context(), body, api.CreateAndroidInstanceParams{
				Wait: api.NewOptBool(true),
			})
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
			if err := json.NewEncoder(w).Encode(instance); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
		}),
	}
	log.Printf("Listening on %s", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
