/*
Package task provides task management, DAG tracking, and semantic search algorithms.
The semantic.go file implements Nomic vector embeddings, cosine similarity metrics,
Bag-of-Words fallback algorithms, and duplicate ticket detection.
*/
package task

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
)

// EmbeddingRequest represents a JSON request payload sent to the Nomic embeddings API.
type EmbeddingRequest struct {
	// Input is the raw text string to convert into a dense float vector embedding.
	Input string `json:"input"`
}

// EmbeddingResponse represents the JSON response payload returned by the embeddings API.
type EmbeddingResponse struct {
	Data []struct {
		// Embedding is the multidimensional float vector representing the semantic text.
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// getEmbedding calls the local Nomic LLM embeddings endpoint (localhost:8081) to retrieve
// the semantic floating point vector for the provided text string.
// It returns a float32 array representing the vector or an error if HTTP communication fails.
func getEmbedding(text string) ([]float32, error) {
	// Marshal the raw text input string into an HTTP POST JSON payload
	reqBody := EmbeddingRequest{Input: text}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// Dispatch HTTP POST request to the local Nomic embedding server
	resp, err := http.Post("http://localhost:8081/v1/embeddings", "application/json", bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check response status code for non-200 HTTP API errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API error: %s", body)
	}

	// Decode JSON embedding vector array from API response stream
	var embResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, err
	}

	// Verify that at least one vector embedding data element was returned
	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return embResp.Data[0].Embedding, nil
}

// cosineSimilarity calculates the mathematical dot product and Euclidean magnitude vector product
// between two multidimensional float32 embedding arrays.
// It returns a similarity score normalized between 0.0 (orthogonal/unrelated) and 1.0 (identical direction).
func cosineSimilarity(t1, t2 []float32) float64 {
	dotProduct := 0.0
	mag1 := 0.0
	mag2 := 0.0

	// Accumulate dot product and vector magnitude components synchronously across vector dimensions
	for i := 0; i < len(t1); i++ {
		v1 := t1[i]
		v2 := t2[i]
		dotProduct += float64(v1 * v2)
		mag1 += float64(v1 * v1)
		mag2 += float64(v2 * v2)
	}

	// Prevent division by zero if either vector has zero Euclidean length
	if mag1 == 0 || mag2 == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(mag1) * math.Sqrt(mag2))
}

// fallbackTextCosineSimilarity calculates a basic Bag-of-Words (Term Frequency) cosine similarity
// between two raw text strings when the neural network embeddings API is unreachable or returns errors.
func fallbackTextCosineSimilarity(s1, s2 string) float64 {
	// Extract normalized lowercase word tokens from input strings
	words1 := strings.Fields(strings.ToLower(s1))
	words2 := strings.Fields(strings.ToLower(s2))

	// Return zero similarity if either string contains no word tokens
	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	// Build term frequency histograms for both text documents
	counts1 := make(map[string]int)
	counts2 := make(map[string]int)

	for _, w := range words1 {
		counts1[w]++
	}
	for _, w := range words2 {
		counts2[w]++
	}

	// Collect unique vocabulary union set across both documents
	allWords := make(map[string]bool)
	for w := range counts1 {
		allWords[w] = true
	}
	for w := range counts2 {
		allWords[w] = true
	}

	// Calculate dot product and term frequency vector lengths
	var dotProduct, mag1, mag2 float64
	for w := range allWords {
		v1 := float64(counts1[w])
		v2 := float64(counts2[w])
		dotProduct += v1 * v2
		mag1 += v1 * v1
		mag2 += v2 * v2
	}

	// Return zero if term frequency magnitudes are zero
	if mag1 == 0 || mag2 == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(mag1) * math.Sqrt(mag2))
}

// buildTaskEmbeddings iterates through all provided tasks and queries the embedding API
// for tasks that are currently in BACKLOG status. It returns a map of token embeddings
// indexed by task key and a slice of active tasks that were successfully retrieved.
func buildTaskEmbeddings(tasks []Task) (map[string][]float32, []Task) {
	tokenMaps := make(map[string][]float32)
	var activeTasks []Task

	// Filter tasks in BACKLOG status and fetch neural embeddings
	for _, t := range tasks {
		if t.Status == StatusBacklog {
			activeTasks = append(activeTasks, t)
			emb, err := getEmbedding(t.Title + "\n" + t.Description)
			if err != nil {
				fmt.Printf("⚠️  Failed to get embedding for Task %s: %v\n", t.Key, err)
				continue
			}
			tokenMaps[t.Key] = emb
		}
	}
	return tokenMaps, activeTasks
}

// isDuplicateTaskPair evaluates whether two tasks are potential duplicates within the same project boundary.
// It checks neural vector similarity (threshold > 0.90) or falls back to text cosine similarity (threshold > 0.75).
func isDuplicateTaskPair(t1, t2 Task, tokenMaps map[string][]float32) bool {
	// Only compare tasks within the same project boundary namespace
	if t1.Project != t2.Project {
		return false
	}

	// Retrieve pre-calculated embeddings for task pair
	emb1, ok1 := tokenMaps[t1.Key]
	emb2, ok2 := tokenMaps[t2.Key]
	if !ok1 || !ok2 {
		// Fallback to text-based cosine similarity if embedding is unavailable
		textSim := fallbackTextCosineSimilarity(t1.Title+"\n"+t1.Description, t2.Title+"\n"+t2.Description)
		return textSim > 0.75
	}

	// Calculate neural vector cosine similarity score
	sim := cosineSimilarity(emb1, emb2)
	return sim > 0.90
}

// detectDuplicates performs an upper-triangular pair comparison of all active tasks within the project.
// It delegates pairwise similarity evaluation to isDuplicateTaskPair to maintain low cyclomatic complexity.
func detectDuplicates(tasks []Task) [][]string {
	var duplicates [][]string
	tokenMaps, activeTasks := buildTaskEmbeddings(tasks)

	// Iterate through pairwise upper-triangular combinations to detect duplicates
	for i := 0; i < len(activeTasks); i++ {
		for j := i + 1; j < len(activeTasks); j++ {
			t1 := activeTasks[i]
			t2 := activeTasks[j]

			// Delegate pair evaluation to helper function to keep complexity low
			if isDuplicateTaskPair(t1, t2, tokenMaps) {
				duplicates = append(duplicates, []string{t1.Key, t2.Key})
			}
		}
	}
	return duplicates
}

// filterActiveProjectTasks returns only tasks in BACKLOG or TRIAGE for the specified project.
// This narrows down the search space for duplicate checking to currently relevant issues.
func filterActiveProjectTasks(tasks []Task, project string) []Task {
	var activeTasks []Task
	for _, t := range tasks {
		if t.Status == StatusBacklog || t.Status == StatusTriage {
			if project == "" || t.Project == project {
				activeTasks = append(activeTasks, t)
			}
		}
	}
	return activeTasks
}

// computeDuplicatesTextFallback iteratively compares newText against active tasks using text cosine similarity.
// It returns a list of task IDs that exceed the 0.75 similarity threshold.
func computeDuplicatesTextFallback(newText string, activeTasks []Task) []string {
	var duplicates []string
	for _, t := range activeTasks {
		sim := fallbackTextCosineSimilarity(newText, t.Title+"\n"+t.Description)
		if sim > 0.75 {
			duplicates = append(duplicates, t.Key)
		}
	}
	return duplicates
}

// CheckDuplicateNewTask checks if a new task description is highly similar to any existing active tasks.
// It prioritizes the Nomic embeddings API (port 8081) and falls back to text cosine similarity if unavailable.
func CheckDuplicateNewTask(ctx context.Context, tracker Tracker, title, description, project string) ([]string, error) {
	// Fetch all tasks from current project tracker
	allTasks, err := tracker.List(ctx)
	if err != nil {
		return nil, err
	}

	// Filter tasks down to active issues in BACKLOG/TRIAGE status
	activeTasks := filterActiveProjectTasks(allTasks, project)
	newText := title + "\n" + description
	newEmb, embErr := getEmbedding(newText)

	// If the primary embedding API fails for the new task, fallback to pure text comparison
	if embErr != nil {
		return computeDuplicatesTextFallback(newText, activeTasks), nil
	}

	// Perform pairwise comparison using vector cosine similarity
	var duplicates []string
	for _, t := range activeTasks {
		emb, err := getEmbedding(t.Title + "\n" + t.Description)
		if err != nil {
			// Fallback for this specific task comparison if its individual embedding fails
			sim := fallbackTextCosineSimilarity(newText, t.Title+"\n"+t.Description)
			if sim > 0.75 {
				duplicates = append(duplicates, t.Key)
			}
			continue
		}
		if cosineSimilarity(newEmb, emb) > 0.90 {
			duplicates = append(duplicates, t.Key)
		}
	}
	return duplicates, nil
}

// TaskSearchResult represents a task and its semantic similarity score to a search query string.
type TaskSearchResult struct {
	// Task is the matching task entity from the DAG database.
	Task Task
	// Score is the floating point similarity score (higher is more relevant).
	Score float64
}

// SemanticSearch queries the local Nomic embeddings API for the search query and scores
// all provided tasks using cosine similarity, returning a sorted list of results in descending order.
func SemanticSearch(query string, tasks []Task) []TaskSearchResult {
	var results []TaskSearchResult

	// Obtain vector embedding for search query string
	queryEmb, embErr := getEmbedding(query)
	useTextFallback := embErr != nil

	// Calculate similarity score for each candidate task
	for _, t := range tasks {
		score := 0.0
		if useTextFallback {
			score = fallbackTextCosineSimilarity(query, t.Title+"\n"+t.Description)
		} else {
			taskEmb, err := getEmbedding(t.Title + "\n" + t.Description)
			if err != nil {
				score = fallbackTextCosineSimilarity(query, t.Title+"\n"+t.Description)
			} else {
				score = cosineSimilarity(queryEmb, taskEmb)
			}
		}

		results = append(results, TaskSearchResult{
			Task:  t,
			Score: score,
		})
	}

	// Sort search results by Score in descending order
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}
