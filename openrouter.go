package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const OpenRouterAPIURL = "https://openrouter.ai/api/v1/chat/completions"

// const OpenRouterModel = "google/gemini-flash-1.5-8b"
const OpenRouterModel = "google/gemini-2.0-flash-001"

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

// AnswerResult - Structura pentru rezultatul răspunsului AI
type AnswerResult struct {
	Answer      string `json:"answer"`
	FoundAnswer bool   `json:"foundAnswer"`
}

// sanitizeJSONString escapes raw control characters (newline, carriage return, tab)
// that may appear unescaped inside JSON string literals returned by the model.
// It walks the input and only replaces these characters when inside a quoted string.
func sanitizeJSONString(s string) string {
	var b strings.Builder
	inString := false
	prevBackslash := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if c == '"' && !prevBackslash {
			inString = !inString
			b.WriteByte(c)
			prevBackslash = false
			continue
		}

		if inString {
			if c == '\n' {
				b.WriteString("\\n")
				prevBackslash = false
				continue
			}
			if c == '\r' {
				b.WriteString("\\r")
				prevBackslash = false
				continue
			}
			if c == '\t' {
				b.WriteString("\\t")
				prevBackslash = false
				continue
			}
			// handle backslash state
			if c == '\\' && !prevBackslash {
				prevBackslash = true
				b.WriteByte(c)
				continue
			}
			// if previous byte was backslash, reset state after consuming
			if prevBackslash {
				prevBackslash = false
				b.WriteByte(c)
				continue
			}

			b.WriteByte(c)
			continue
		}

		// outside string, just copy
		b.WriteByte(c)
		prevBackslash = false
	}

	return b.String()
}

// answerFromVectorDB - Răspunde la întrebări pe baza JSON-ului din Qdrant
func answerFromVectorDB(question string, openRouterAnswerLanguage string, vectorDBResults string) (*AnswerResult, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	prompt := fmt.Sprintf(`Îți trimit fragmente din vectorDB bazate pe întrebarea aceasta. Trebuie să răspunzi la întrebare pe baza informațiilor din JSON.
FOARTE IMPORTANT - NU MENTIONA DE VECTORDB. POTI MENTIONA DOAR CA NU AI GASIT IN DOCUMENTE, TE REFERI LA DOCUMENTE, NICIODATA LA VECTORDB, DAR NU REPETA LA FIECARE PROPOZITIE DOCUMENTE, VORBESTE IMPRESIONAL, PROFESIONIST
FOARTE IMPORTANT - LIMBA RĂSPUNSULUI: %s
FOARTE IMPORTANT - RASPUNSUL TREBUIE SA FIE NUANȚAT, PROFESIONIST, COMPLET ȘI DETALIAT.

Dacă limba detectată este "romanian":
- Răspunsul TREBUIE să fie în română completă
- Folosește diacritice corecte (ă, î, â, ș, ț)
- Exemplu de început: "Pe baza documentelor furnizate, pot să spun că..."

Dacă limba detectată este "english":
- Răspunsul TREBUIE să fie în engleză
- Exemplu de început: "Based on the provided documents..."

Question: %s

Analizează dacă informațiile din vectorDB pot răspunde la întrebare. Returneaza DOAR un JSON în următorul format (fără markdown):

{
  "answer": "răspunsul tău profesional în limba specificată",
  "foundAnswer": true/false (true dacă ai găsit informații relevante, false dacă nu),
}

VectorDB Results (JSON):
%s`, openRouterAnswerLanguage, question, vectorDBResults)

	reqBody := OpenRouterRequest{
		Model: OpenRouterModel,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Tu ești un asistent care răspunde strict în limba specificată și returnează JSON. Limba: %s", openRouterAnswerLanguage),
			},
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
	cleanResponse = strings.TrimPrefix(cleanResponse, "```json")
	cleanResponse = strings.TrimSuffix(cleanResponse, "```")
	cleanResponse = strings.ReplaceAll(cleanResponse, "```", "")
	cleanResponse = strings.TrimSpace(cleanResponse)

	// Parse JSON response (sanitize unescaped control chars first)
	var result AnswerResult
	sanitized := sanitizeJSONString(cleanResponse)
	err = json.Unmarshal([]byte(sanitized), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AI response as JSON: %v. Response was: %s", err, cleanResponse)
	}

	return &result, nil
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
	cleanResponse = strings.TrimPrefix(cleanResponse, "```json")
	cleanResponse = strings.TrimSuffix(cleanResponse, "```")
	cleanResponse = strings.ReplaceAll(cleanResponse, "```", "")
	cleanResponse = strings.TrimSpace(cleanResponse)

	// Parse JSON response (sanitize unescaped control chars first)
	var result KeywordExtractionResult
	sanitized := sanitizeJSONString(cleanResponse)
	err = json.Unmarshal([]byte(sanitized), &result)
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

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", OpenRouterAPIURL, bytes.NewBuffer(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/catalinfl/pdf-response")
	req.Header.Set("X-Title", "PDF Response Tool")

	// Client with timeout
	client := &http.Client{
		Timeout: 35 * time.Second, // Slightly higher than context timeout
	}

	startTime := time.Now()
	resp, err := client.Do(req)
	apiCallDuration := time.Since(startTime)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("OpenRouter API call timeout after %v", apiCallDuration)
		}
		return "", fmt.Errorf("failed to call OpenRouter API: %v (took %v)", err, apiCallDuration)
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
