package main

import (
	"log"
	"os"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"duragraph/workers/go-adapter/pkg/activities"
)

func main() {
	hostPort := os.Getenv("TEMPORAL_HOSTPORT")
	if hostPort == "" {
		hostPort = "localhost:7233"
	}
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	c, err := client.Dial(client.Options{
		HostPort:  hostPort,
		Namespace: namespace,
	})
	if err != nil {
		log.Fatalf("unable to create Temporal client: %v", err)
	}
	defer c.Close()

	w := worker.New(c, "go-adapter", worker.Options{})
	w.RegisterActivity(activities.LLMCallActivity)
	w.RegisterActivity(activities.ToolActivity)

	log.Println("Starting Go adapter worker on task queue 'go-adapter'")
	err = w.Run(worker.InterruptCh())
	if err != nil {
		log.Fatalf("unable to start worker: %v", err)
	}
}
