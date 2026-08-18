package main
import (
"fmt"
"os"
"strings"
)

func isCriterionCovered(criterion, text string) bool {
	cleanCriterion := strings.ReplaceAll(criterion, "`", " ")
cleanText := strings.ReplaceAll(text, "`", " ")

	words := strings.Fields(strings.ToLower(cleanCriterion))
	textLower := strings.ToLower(cleanText)

	if len(words) == 0 {
		return true
	}

	matchCount := 0
	for _, w := range words {
		if len(w) < 4 {
			continue
		}
		if strings.Contains(textLower, w) {
			matchCount++
		}
	}

	return matchCount > 0
}

func main() {
	b, _ := os.ReadFile("/home/markg/.gemini/antigravity-ide/brain/a1bce556-7909-4356-bf22-84a4b85bd27b/walkthrough.md")
	text := string(b)
	fmt.Println("Result:", isCriterionCovered("- [ ] Existing TDD specs pass.", text))
}
