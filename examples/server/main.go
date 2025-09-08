package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
)

func main() {
	token := os.Getenv("LIM_TOKEN") // lim_yourtoken
	lim := limrun.NewClient(option.WithAPIKey(token))

	s := http.Server{
		Addr: ":3000",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			spec := limrun.AndroidInstanceNewParamsSpec{}
			clientIp := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
			if clientIp != "" {
				log.Printf("Using client IP %s as scheduling clue", clientIp)
				spec.Clues = append(spec.Clues, limrun.AndroidInstanceNewParamsSpecClue{
					Kind:     "ClientIP",
					ClientIP: param.NewOpt(clientIp),
				})
			} else {
				log.Println("No client IP specified as scheduling clue")
			}
			instance, err := lim.AndroidInstances.New(r.Context(), limrun.AndroidInstanceNewParams{
				Spec: spec,
				Wait: param.NewOpt(true),
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
