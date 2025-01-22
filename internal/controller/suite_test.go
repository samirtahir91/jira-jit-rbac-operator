/*
Copyright 2024.

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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	jira "github.com/ctreminiom/go-atlassian/v2/jira/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	justintimev1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/internal/config"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc
var ts *httptest.Server

// Structs for mocking jira
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

// issues map globally for mocking jira
var issues = make(map[string]*Issue)
var users = map[string]User{
	"master-chief@unsc.com": {Name: "john117"},
	"cpt-keyes@unsc.com":    {Name: "cptKeyes"},
}

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = justintimev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Register and start the controller
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	// Start the stub server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}))

	// Jira client
	jiraBaseUrl := ts.URL
	jiraClient, err := jira.New(nil, jiraBaseUrl)
	Expect(err).ToNot(HaveOccurred())
	jiraClient.Auth.SetBearerToken("dummy")

	err = (&JitRequestReconciler{
		JiraClient: jiraClient,
		Client:     k8sManager.GetClient(),
		Scheme:     k8sManager.GetScheme(),
		Recorder:   k8sManager.GetEventRecorderFor("jitrequest-controller"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&config.JustInTimeConfigReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager, TestJitConfig, "/tmp/jit-test")
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	// Stop stub server
	ts.Close()
})

// mock jira api calls
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
	// Get the issue key from URL
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
		// Create the comment response
		commentResponse := Comment{
			ID:      "10000",
			Body:    req.Body,
			Author:  Author{Name: "mockuser"},
			Created: "2025-01-20T21:01:46.000+0000",
		}
		// Return response
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
	// Get issue key from the URL path
	issueKey := r.URL.Path[len("/rest/api/2/issue/"):]

	if issue, ok := issues[issueKey]; ok {
		w.WriteHeader(http.StatusOK)
		// Create issue response
		issueResponse := Issue{
			ID:   "10000",
			Key:  issue.Key,
			Self: fmt.Sprintf("%s/rest/api/2/issue/%s", ts.URL, issue.Key),
			Fields: Fields{
				Summary: issue.Fields.Summary,
				Status: Status{
					Name: TestJiraWorkflowApproveStatus, // set in test
				},
			},
		}
		// Return response
		if err := json.NewEncoder(w).Encode(issueResponse); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	} else {
		http.Error(w, "issue not found", http.StatusNotFound)
	}
}
