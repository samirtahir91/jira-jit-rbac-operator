package utils

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
)

type Issue struct {
	ID       string   `json:"id"`
	Key      string   `json:"key"`
	Self     string   `json:"self"`
	Fields   Fields   `json:"fields"`
	Comments []string `json:"comments"`
}

type Fields struct {
	Summary string `json:"summary"`
	Status  Status `json:"status"`
}

type Status struct {
	Name string `json:"name"`
}

type User struct {
	Name string `json:"name"`
}

type Comment struct {
	ID      string `json:"id"`
	Body    string `json:"body"`
	Author  Author `json:"author"`
	Created string `json:"created"`
}

type Author struct {
	Name string `json:"name"`
}

var IssueStatus string
var issues = make(map[string]*Issue)
var users = map[string]User{
	"master-chief@unsc.com": {Name: "john117"},
	"cpt-keyes@unsc.com":    {Name: "cptKeyes"},
}

func CreateHTTPServer() *httptest.Server {
	// Create a listener on specific port
	listener, err := net.Listen("tcp", ":8082")
	if err != nil {
		panic(err) // Handle error appropriately in your code
	}

	// handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if r.URL.Path == "/rest/api/2/issue" {
				createIssue(w, r)
			} else if strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/") && strings.HasSuffix(r.URL.Path, "/comment") {
				addComment(w, r)
			}
		case http.MethodPut:
			if r.URL.Path == "/rest/api/2/issue/transition/rejected" {
				transitionIssue(w, r, "rejected")
			} else if r.URL.Path == "/rest/api/2/issue/transition/completed" {
				transitionIssue(w, r, "completed")
			}
		case http.MethodGet:
			if strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/") {
				getIssueDetails(w, r)
			} else if r.URL.Path == "/rest/api/2/user/search" {
				getUserByEmail(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})

	// start server
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()

	return server
}

// func CreateHTTPServer() *httptest.Server {
// 	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		switch r.Method {
// 		case http.MethodPost:
// 			if r.URL.Path == "/rest/api/2/issue" {
// 				createIssue(w, r)
// 			} else if strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/") && strings.HasSuffix(r.URL.Path, "/comment") {
// 				addComment(w, r)
// 			}
// 		case http.MethodPut:
// 			if r.URL.Path == "/rest/api/2/issue/transition/rejected" {
// 				transitionIssue(w, r, "rejected")
// 			} else if r.URL.Path == "/rest/api/2/issue/transition/completed" {
// 				transitionIssue(w, r, "completed")
// 			}
// 		case http.MethodGet:
// 			if strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/") {
// 				getIssueDetails(w, r)
// 			} else if r.URL.Path == "/rest/api/2/user/search" {
// 				getUserByEmail(w, r)
// 			}
// 		default:
// 			http.NotFound(w, r)
// 		}
// 	}))
// }

func createIssue(w http.ResponseWriter, r *http.Request) {
	var issue Issue
	if err := json.NewDecoder(r.Body).Decode(&issue); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	issue.Key = "IAM-1"
	issues[issue.Key] = &issue
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(issue); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func addComment(w http.ResponseWriter, r *http.Request) {
	issueKey := r.URL.Path[len("/rest/api/2/issue/"):]
	issueKey = issueKey[:len(issueKey)-len("/comment")]

	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if issue, ok := issues[issueKey]; ok {
		issue.Comments = append(issue.Comments, req.Body)
		w.WriteHeader(http.StatusCreated)
		commentResponse := Comment{
			ID:      "10000",
			Body:    req.Body,
			Author:  Author{Name: "mockuser"},
			Created: "2025-01-20T21:01:46.000+0000",
		}
		if err := json.NewEncoder(w).Encode(commentResponse); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	} else {
		http.Error(w, "issue not found", http.StatusNotFound)
	}
}

func transitionIssue(w http.ResponseWriter, r *http.Request, status string) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if issue, ok := issues[req.Key]; ok {
		issue.Fields.Status = Status{Name: status}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(issue); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	} else {
		http.NotFound(w, r)
	}
}

func getUserByEmail(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("username")
	if user, ok := users[email]; ok {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]User{user}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	} else {
		http.NotFound(w, r)
	}
}

func getIssueDetails(w http.ResponseWriter, r *http.Request) {
	issueKey := r.URL.Path[len("/rest/api/2/issue/"):]

	if issue, ok := issues[issueKey]; ok {
		w.WriteHeader(http.StatusOK)
		issueResponse := Issue{
			ID:   "10000",
			Key:  issue.Key,
			Self: fmt.Sprintf("%s/rest/api/2/issue/%s", r.Host, issue.Key),
			Fields: Fields{
				Summary: issue.Fields.Summary,
				Status: Status{
					Name: IssueStatus,
				},
			},
		}
		if err := json.NewEncoder(w).Encode(issueResponse); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	} else {
		http.Error(w, "issue not found", http.StatusNotFound)
	}
}
