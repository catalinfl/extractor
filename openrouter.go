package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const OpenRouterAPIURL = "https://openrouter.ai/api/v1/chat/completions"
const OpenRouterModel = "google/gemini-flash-1.5-8b"

// OpenRouter API structures
type OpenRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []OpenRouterMessage `json:"messages"`
	Temperature float32             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

type OpenRouterChoice struct {
	Message      OpenRouterMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
	Index        int               `json:"index"`
}

type OpenRouterResponse struct {
	ID      string             `json:"id"`
	Choices []OpenRouterChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// KeywordExtractionResult - Structura pentru rezultatul extragerii de cuvinte cheie
type KeywordExtractionResult struct {
	Query    string `json:"query"`
	Language string `json:"language"`
}

// answerFromVectorDB - Răspunde la întrebări pe baza JSON-ului din Qdrant
func answerFromVectorDB(question string, openRouterAnswerLanguage string, vectorDBResults string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	prompt := fmt.Sprintf(`Îți trimit fragmente din vectorDB bazate pe întrebarea aceasta. Trebuie să răspunzi la întrebare DOAR pe baza informațiilor din JSON.

FOARTE IMPORTANT - LIMBA RĂSPUNSULUI: %s

Dacă limba detectată este "romanian":
- Răspunsul TREBUIE să fie în română completă
- Folosește diacritice corecte (ă, î, â, ș, ț)
- Exemplu de început: "Pe baza documentelor furnizate, pot să spun că..."
- NU răspunde în engleză

Dacă limba detectată este "english":
- Răspunsul TREBUIE să fie în engleză
- Exemplu de început: "Based on the provided documents..."

Question: %s

Vreau ca răspunsul să fie foarte profesional și dezvoltat. Răspunde DOAR în limba specificată mai sus.

VectorDB Results (JSON):
%s`, openRouterAnswerLanguage, question, vectorDBResults)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.2,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Tu ești un asistent care răspunde strict în limba specificată. Dacă limba este 'romanian', răspunzi DOAR în română cu diacritice. Dacă limba este 'english', răspunzi DOAR în engleză. Limba pentru acest răspuns: %s", openRouterAnswerLanguage),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	return callOpenRouter(reqBody, apiKey)
}

// extractKeywords - Extrage cuvinte cheie din întrebare pentru căutare în Qdrant
func extractKeywords(question string) (*KeywordExtractionResult, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	prompt := fmt.Sprintf(`Esti un bot care pe baza intrebarii mele faci urmatorul lucru - De asemenea detecteaza limba.
Pun intrebarea, iar tu gasesti cuvintele cheie, practic elimini intrebarea de exemplu:

ANTRENAMENT:
"Ce imi poti spune despre calatoria omului cu vacile?"
Tu imi vei returna cuvintele cheie "calatorie om vaci"

"Ce face Mihai cand se duce dupa nevasta carutasului?"
Tu imi returnezi "Mihai nevasta carutasului"

"Ce fac vacile in cadrul povestii?"
Tu imi returnezi "vaci"

Ce imi returnezi traduci in engleza, plus adaugi vreo 2-3 sinonime, daca sunt valabile, dar tot la fel, nu conteaza ordinea.
Fa asta intr-un mod profesional, deoarece aceste cuvinte le pot folosi pentru cautare intr-un vectorDB (Qdrant)

IMPORTANT: Returneaza DOAR un JSON valid in urmatorul format, fără markdown, fără explicatii, fără code blocks:
{
  "query": "cuvintele cheie traduse in engleza cu sinonime",
  "language": "limba detectata (romanian, english, etc)"
}

Nu folosi formatari markdown precum 'json'. Doar JSON-ul curat.
	
INTREBARE:
%s`, question)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.3,
		Messages: []OpenRouterMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	responseStr, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		return nil, err
	}

	// Clean the response - remove markdown code blocks if present
	cleanResponse := strings.TrimSpace(responseStr)
	
	// Remove ```json at the beginning
	if strings.HasPrefix(cleanResponse, "```json") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "```json")
	}
	
	// Remove ``` at the end
	if strings.HasSuffix(cleanResponse, "```") {
		cleanResponse = strings.TrimSuffix(cleanResponse, "```")
	}
	
	// Remove any remaining ``` patterns
	cleanResponse = strings.ReplaceAll(cleanResponse, "```", "")
	cleanResponse = strings.TrimSpace(cleanResponse)

	// Parse JSON response
	var result KeywordExtractionResult
	err = json.Unmarshal([]byte(cleanResponse), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AI response as JSON: %v. Response was: %s", err, cleanResponse)
	}

	return &result, nil
}

// callOpenRouter - Funcția comună pentru apelurile la OpenRouter API
func callOpenRouter(reqBody OpenRouterRequest, apiKey string) (string, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", OpenRouterAPIURL, bytes.NewBuffer(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/catalinfl/pdf-response")
	req.Header.Set("X-Title", "PDF Response Tool")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenRouter API: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OpenRouter API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var openRouterResp OpenRouterResponse
	if err := json.Unmarshal(bodyBytes, &openRouterResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	if openRouterResp.Error != nil {
		return "", fmt.Errorf("OpenRouter API error: %s", openRouterResp.Error.Message)
	}

	if len(openRouterResp.Choices) == 0 {
		return "", fmt.Errorf("no response choices received")
	}

	answer := strings.TrimSpace(openRouterResp.Choices[0].Message.Content)
	fmt.Printf("🤖 OpenRouter API call completed (tokens: %d)\n", openRouterResp.Usage.TotalTokens)

	return answer, nil
}
