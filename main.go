package main

import (
	"os"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/goji/httpauth"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/rest"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	batch_v1 "k8s.io/client-go/pkg/apis/batch/v1"
)

var flagKubeconfigPath string

var flagInCluster bool

var deletePolicy = meta_v1.DeletePropagationForeground

var authUsername = os.Getenv("GORUNIT_USERNAME")

var authPassword = os.Getenv("GORUNIT_PASSWORD")

func getInClusterConfig() (*rest.Config, error) {
	return rest.InClusterConfig()
}

func getOutOfClusterConfig() (*rest.Config, error) {
	config, err := clientcmd.BuildConfigFromFlags("", flagKubeconfigPath)

	if err != nil {
		return nil, err
	}

	return config, err
}

func getKubeClientset() (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	if flagInCluster {
		config, err = getInClusterConfig()
	} else {
		config, err = getOutOfClusterConfig()
	}

	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func jobLog(job *batch_v1.Job, message string) {
	log.Printf("%s: %s\n", job.ObjectMeta.Name, message)
}

func deleteJob(job *batch_v1.Job) {
	jobLog(job, "cleaning up")

	client, _ := getKubeClientset()

	err := client.BatchV1().Jobs(job.ObjectMeta.Namespace).Delete(
		job.ObjectMeta.Name,
		&meta_v1.DeleteOptions{PropagationPolicy: &deletePolicy})

	if err != nil {
		jobLog(job, "error when trying to remove")
	} else {
		jobLog(job, "removed")
	}
}

func watchAndCleanUpJob(job *batch_v1.Job) {
	defer deleteJob(job)

	for {
		client, _ := getKubeClientset()

		j, err := client.BatchV1().Jobs(job.ObjectMeta.Namespace).Get(
			job.ObjectMeta.Name,
			meta_v1.GetOptions{})

		if err != nil {
			jobLog(job, "something went terribly wrong, can't retrieve job status")
		}

		if j.Status.Active == 1 {
			jobLog(job, "active")
		} else if j.Status.Succeeded == 1 {
			jobLog(job, "finished")
			break
		} else if j.Status.Failed == 1 {
			jobLog(job, "failed")
			break
		} else {
			jobLog(job, "I have no idea what the fuck is happening")
		}

		time.Sleep(time.Second)
	}
}

func getJobFromRequestBody(r *http.Request) (*batch_v1.Job, error) {
	var job batch_v1.Job

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	if err := decoder.Decode(&job); err != nil {
		return nil, err
	}

	return &job, nil
}

func handleCreateJob(w http.ResponseWriter, r *http.Request) {
	client, _ := getKubeClientset()

	job, err := getJobFromRequestBody(r)
	if err != nil {
		fmt.Fprint(w, "failed to parse request body")
		return
	}

	// quick hack to randomize job names a bit
	name := fmt.Sprintf("%s-%s", job.ObjectMeta.Name, uuid.New().String())
	job.ObjectMeta.Name = name
	job.Spec.Template.ObjectMeta.Name = name

	j, err := client.BatchV1().Jobs(job.ObjectMeta.Namespace).Create(job)
	if err != nil {
		jobLog(j, "failed to create")
		return
	}

	jobLog(j, "created")
	defer func() {
		go watchAndCleanUpJob(j)
	}()
}

func init() {
	flag.StringVar(
		&flagKubeconfigPath,
		"kubeconfig",
		"kubeconfig",
		"Absolute path to the kubeconfig file.")

	flag.BoolVar(
		&flagInCluster,
		"in-cluster",
		false,
		"Running inside Kubernetes cluster")

	flag.Parse()
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Go run it!")
}

func handleKubePing(w http.ResponseWriter, r *http.Request) {
	client, _ := getKubeClientset()

	pods, err := client.CoreV1().Pods("").List(meta_v1.ListOptions{})
	if err != nil {
		fmt.Fprint(w, "can't talk to the cluster")
	} else {
		fmt.Fprintf(w, "%d pods running\n", len(pods.Items))
	}
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/v1/ping", handleKubePing).Methods("GET")
	r.HandleFunc("/v1/jobs", handleCreateJob).Methods("POST")
	r.HandleFunc("/", handleHome).Methods("GET")

	var handler http.Handler
	if len(authUsername) != 0 && len(authPassword) != 0 {
		handler = httpauth.SimpleBasicAuth(authUsername, authPassword)(r)
	} else {
		handler = r
	}

	s := &http.Server{
		Handler:        handler,
		Addr:           ":10777",
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	log.Println("Listening on :10777")
	log.Fatal(s.ListenAndServe())
}
