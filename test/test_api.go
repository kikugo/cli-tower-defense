package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file:", err)
		return
	}

	// Get API keys
	openaiKey := os.Getenv("OPENAI_API_KEY")
	googleKey := os.Getenv("GOOGLE_API_KEY")

	fmt.Println("\nAPI Key Check:")
	fmt.Println("==============")
	
	// Check if keys are set
	if openaiKey == "" {
		fmt.Println("❌ OpenAI API key is not set")
	} else {
		fmt.Printf("✓ OpenAI API key found (length: %d)\n", len(openaiKey))
		testOpenAI(openaiKey)
	}

	if googleKey == "" {
		fmt.Println("❌ Google API key is not set")
	} else {
		fmt.Printf("✓ Google API key found (length: %d)\n", len(googleKey))
		testGemini(googleKey)
	}
}

func testOpenAI(apiKey string) {
	fmt.Println("\nTesting OpenAI API...")
	
	// Create a simple request
	reqBody := map[string]interface{}{
		"model": "gpt-4o-mini-2024-07-18",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Say hello in one word"},
		},
		"temperature": 0.7,
		"max_tokens":  10,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Println("❌ Error creating request:", err)
		return
	}

	// Create and send the request
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqJSON))
	if err != nil {
		fmt.Println("❌ Error creating HTTP request:", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ Error making request:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✓ OpenAI API test successful!")
	} else {
		fmt.Printf("❌ OpenAI API test failed with status code: %d\n", resp.StatusCode)
		
		// Read and print error response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			if error, ok := result["error"].(map[string]interface{}); ok {
				fmt.Printf("Error message: %v\n", error["message"])
			}
		}
	}
}

func testGemini(apiKey string) {
	fmt.Println("\nTesting Gemini API...")
	
	// Create a simple request
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": "Say hello in one word"},
				},
			},
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Println("❌ Error creating request:", err)
		return
	}

	// Create and send the request
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqJSON))
	if err != nil {
		fmt.Println("❌ Error creating HTTP request:", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ Error making request:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✓ Gemini API test successful!")
	} else {
		fmt.Printf("❌ Gemini API test failed with status code: %d\n", resp.StatusCode)
		
		// Read and print error response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			if error, ok := result["error"].(map[string]interface{}); ok {
				fmt.Printf("Error message: %v\n", error["message"])
			}
		}
	}
}
