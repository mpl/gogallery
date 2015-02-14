package main

import (
	"http"
)

const (
	host = "smgl.fr.eu.org:8081"
	redirected = "https://smgl.fr.eu.org/"
)

func redirectToHttps(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, redirected, http.StatusFound)
}

func main() {
	http.HandleFunc("/", http.HandlerFunc(redirectToHttps))
	http.ListenAndServe(host, nil)
}
