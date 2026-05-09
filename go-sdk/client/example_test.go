package client_test

import (
	"context"
	"fmt"
	"time"

	"github.com/duragraph/duragraph-go/client"
)

func ExampleNew() {
	c := client.New("http://localhost:8081")
	_ = c // use client
}

func ExampleNew_withAuth() {
	c := client.New("http://localhost:8081", client.WithAPIKey("sk-your-key"))
	_ = c // use client
}

func ExampleClient_CreateAssistant() {
	c := client.New("http://localhost:8081")
	assistant, err := c.CreateAssistant(context.Background(), client.CreateAssistantRequest{
		GraphID: "chatbot",
		Name:    "My Agent",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("Created assistant:", assistant.ID)
}

func ExampleClient_CreateThread() {
	c := client.New("http://localhost:8081")
	thread, err := c.CreateThread(context.Background(), client.CreateThreadRequest{
		Metadata: map[string]any{"user_id": "u-123"},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("Created thread:", thread.ID)
}

func ExampleClient_CreateRun() {
	c := client.New("http://localhost:8081")
	run, err := c.CreateRun(context.Background(), "thread-id", client.CreateRunRequest{
		AssistantID: "assistant-id",
		Input:       map[string]any{"message": "Hello!"},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("Run status:", run.Status)
}

func ExampleClient_WaitForRun() {
	c := client.New("http://localhost:8081")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	run, err := c.WaitForRun(ctx, "thread-id", "run-id", time.Second)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("Final status:", run.Status)
}

func ExampleClient_PutStoreItem() {
	c := client.New("http://localhost:8081")
	err := c.PutStoreItem(context.Background(), client.PutStoreItemRequest{
		Namespace: []string{"users", "prefs"},
		Key:       "user-123",
		Value:     map[string]any{"theme": "dark"},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("Stored item")
}

func ExampleClient_CreateCron() {
	c := client.New("http://localhost:8081")
	cron, err := c.CreateCron(context.Background(), client.CreateCronRequest{
		AssistantID: "assistant-id",
		Schedule:    "0 */6 * * *",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("Created cron:", cron.CronID)
}

func ExampleClient_GetThreadState() {
	c := client.New("http://localhost:8081")
	state, err := c.GetThreadState(context.Background(), "thread-id")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("State values:", state.Values)
}
