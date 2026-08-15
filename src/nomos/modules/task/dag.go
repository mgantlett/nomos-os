package task

// GetActiveContextBatch filters the provided tasks and returns a slice of tasks
// that represent the current active topological batch.
// A task is considered part of the active batch if it is open (not DONE or CANCELLED)
// and all of its dependencies (BlockedBy) are in a terminal state or missing.
func GetActiveContextBatch(tasks []Task) []Task {
	// Build a lookup map to efficiently check the status of dependencies
	statusMap := make(map[string]TaskStatus)
	for _, t := range tasks {
		statusMap[t.Key] = t.Status
	}

	var batch []Task
	for _, t := range tasks {
		// Only consider open tasks
		if t.IsClosed() {
			continue
		}

		isUnblocked := true
		for _, depKey := range t.BlockedBy {
			depStatus, exists := statusMap[depKey]
			// If dependency exists and is not closed, the task is blocked
			if exists && depStatus != StatusDone && depStatus != StatusCancelled {
				isUnblocked = false
				break
			}
			// If dependency doesn't exist, we treat it as fulfilled to prevent permanent locks
		}

		if isUnblocked {
			batch = append(batch, t)
		}
	}

	return batch
}
