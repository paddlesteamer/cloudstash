package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/icza/gox/osx"
)

const (
	dropboxOAuth2URLTemplate = "https://www.dropbox.com/oauth2/authorize?client_id=%s&response_type=token&redirect_uri=%s"
	dropboxRedirectURI       = "http://localhost:48500/dbx/redirect"
)

const htmlTemplate = `
<html><head><script>

var post = function(result) {
	console.log("here")
	var http = new XMLHttpRequest();
	var url = "/dbx/token";

	http.open("POST", url, true);

	//Send the proper header information along with the request
	http.setRequestHeader("Content-type", "application/json");

	http.onreadystatechange = function() {
		if (this.readyState == 4) {
			window.location.href = "https://www.dropbox.com/";
		}
	};

	http.send(JSON.stringify(result));
};

var hash = window.location.hash;
var atIdx = hash.indexOf("access_token");

if (atIdx === -1) {
	var result = {
		status: 0
	}

	post(result)
} else {

	var start = atIdx + "access_token=".length;
	var end = hash.indexOf("&", start);

	var token = hash.substring(start, end);

	var result = {
		status: 1,
		token: token
	}

	post(result);
}

</script></head><html>
` // pure javascript

const errNotAuthorized string = "notauthorized"

type result struct {
	Status int
	Token  string
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, htmlTemplate)
}

func tokenHandler(ch chan string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		buffer := make([]byte, 256)
		n, _ := io.ReadFull(r.Body, buffer)

		postBody := buffer[:n]

		var res result
		json.Unmarshal(postBody, &res)

		if res.Status == 0 {
			ch <- errNotAuthorized
			return
		}

		ch <- res.Token
	})
}

func serve(wg *sync.WaitGroup, ch chan string) *http.Server {
	srv := &http.Server{
		Addr: "localhost:48500",
	}

	http.HandleFunc("/dbx/redirect", redirectHandler)

	http.Handle("/dbx/token", tokenHandler(ch))

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("http server is crashed: %v", err)
		}
	}()

	return srv
}

func GetDropboxToken(appkey string) (string, error) {
	ch := make(chan string)
	wg := &sync.WaitGroup{}

	srv := serve(wg, ch)

	osx.OpenDefault(fmt.Sprintf(dropboxOAuth2URLTemplate, appkey, url.QueryEscape(dropboxRedirectURI)))

	token := <-ch

	if err := srv.Shutdown(context.Background()); err != nil {
		return "", fmt.Errorf("couldn't shutdown http server: %v", err)
	}

	wg.Wait()

	if token == errNotAuthorized {
		return "", fmt.Errorf("app is not authorized")
	}

	return token, nil
}
