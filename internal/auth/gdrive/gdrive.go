package gdrive

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/icza/gox/osx"
	"github.com/paddlesteamer/cloudstash/internal/auth"
	"golang.org/x/oauth2"
)

func redirectHandler(ch chan string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		code, present := query["code"]

		if !present || len(code) != 1 {
			ch <- auth.ErrNotAuthorized
			return
		}

		ch <- code[0]

		http.Redirect(w, r, "https://wwww.drive.google.com", http.StatusFound)
	})
}

func serve(wg *sync.WaitGroup, ch chan string) *http.Server {
	srv := &http.Server{
		Addr: auth.ListenAddr,
	}

	http.Handle("/gdrive/redirect", redirectHandler(ch))

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("http server is crashed: %v", err)
		}
	}()

	return srv
}

func GetToken(config *oauth2.Config) (*oauth2.Token, error) {
	ch := make(chan string)
	wg := &sync.WaitGroup{}

	srv := serve(wg, ch)

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	osx.OpenDefault(authURL)

	code := <-ch

	if err := srv.Shutdown(context.Background()); err != nil {
		return nil, fmt.Errorf("couldn't shutdown http server: %v", err)
	}

	wg.Wait()

	if code == auth.ErrNotAuthorized {
		return nil, fmt.Errorf("gdrive isn't authorized")
	}

	token, err := config.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("couldn't get gdrive token: %v", err)
	}

	return token, nil
}
