//go:build ignore
// +build ignore

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
	}

	// Get API keys
	openaiKey := os.Getenv("OPENAI_API_KEY")
	googleKey := os.Getenv("GOOGLE_API_KEY")

	fmt.Println("API Key Check:")
	fmt.Println("==============")

	// Check if keys are set
	if openaiKey == "" {
		fmt.Println("❌ OpenAI API key is not set")
	} else {
		fmt.Printf("✓ OpenAI API key found (starts with: %s...)\n", openaiKey[:5])
		testOpenAI(openaiKey)
	}

	if googleKey == "" {
		fmt.Println("❌ Google API key is not set")
	} else {
		fmt.Printf("✓ Google API key found (starts with: %s...)\n", googleKey[:5])
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

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ Error connecting to OpenAI API:", err)
		return
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode == 200 {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			fmt.Println("❌ Error decoding response:", err)
			return
		}

		// Extract the response
		choices, ok := result["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			fmt.Println("❌ Invalid response format")
			return
		}

		choice := choices[0].(map[string]interface{})
		message := choice["message"].(map[string]interface{})
		content := message["content"].(string)

		fmt.Printf("✓ OpenAI API response: %s\n", content)
	} else {
		fmt.Printf("❌ OpenAI API error: HTTP status %d\n", resp.StatusCode)
		var errorResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
			fmt.Println("Error details:", errorResp)
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
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 10,
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Println("❌ Error creating request:", err)
		return
	}

	// Create and send the request
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey),
		bytes.NewBuffer(reqJSON))
	if err != nil {
		fmt.Println("❌ Error creating HTTP request:", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ Error connecting to Gemini API:", err)
		return
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode == 200 {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			fmt.Println("❌ Error decoding response:", err)
			return
		}

		// Extract the response
		candidates, ok := result["candidates"].([]interface{})
		if !ok || len(candidates) == 0 {
			fmt.Println("❌ Invalid response format")
			return
		}

		candidate := candidates[0].(map[string]interface{})
		content := candidate["content"].(map[string]interface{})
		parts := content["parts"].([]interface{})
		text := parts[0].(map[string]interface{})["text"].(string)

		fmt.Printf("✓ Gemini API response: %s\n", text)
	} else {
		fmt.Printf("❌ Gemini API error: HTTP status %d\n", resp.StatusCode)
		var errorResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
			fmt.Println("Error details:", errorResp)
		}
	}
}
