package translator

import "log"

// HardcodedWorkflow is a minimal translator that maps input into a 3-step run:
// 1. input -> 2. llm_call (echo) -> 3. end
func HardcodedWorkflow(input string) []string {
	log.Printf("[translator] received input: %s", input)
	steps := []string{
		"input",
		"llm_call: echo(" + input + ")",
		"end",
	}
	log.Printf("[translator] steps: %v", steps)
	return steps
}
