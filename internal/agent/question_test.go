package agent

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAskQuestionWaitsForAValidAnswer(t *testing.T) {
	runtime := newTestRuntime(t)
	if err := RegisterAskQuestion(runtime.Executor.Registry); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	question := `{"questions":[{"question":"Which window?","options":[{"label":"1h"},{"label":"24h","description":"The console's default"}]},{"question":"Anything to exclude?"}]}`
	for _, raw := range []string{
		`{"questions":[]}`,
		`{"questions":[{"question":"Pick one","options":[{"label":"only"}]}]}`,
		`{"questions":[{"question":"Pick","options":[{"label":"a"},{"label":"a"}]}]}`,
		`{"questions":[{"question":"Free","multi_select":true}]}`,
	} {
		if result, err := runtime.Executor.Invoke(ctx, PRINCIPAL, ASK_QUESTION, json.RawMessage(raw), ""); err == nil {
			t.Fatalf("accepted %s: %+v", raw, result)
		}
	}
	result, err := runtime.Executor.Invoke(ctx, PRINCIPAL, ASK_QUESTION, json.RawMessage(question), "")
	if err != nil || result.Status != "pending" {
		t.Fatalf("ask: %+v %v", result, err)
	}
	for _, answer := range []string{
		``,
		`{"answers":[{"selected":["1h"]}]}`,
		`{"answers":[{"selected":["1h","24h"]},{"text":"none"}]}`,
		`{"answers":[{"selected":["7d"]},{"text":"none"}]}`,
		`{"answers":[{"selected":["1h"]},{"text":"  "}]}`,
	} {
		if _, err := runtime.Executor.Decide(ctx, PRINCIPAL, result.OperationID, true, answer); err == nil {
			t.Fatalf("accepted answer %q", answer)
		}
	}
	operation, err := runtime.Executor.Get(ctx, PRINCIPAL, result.OperationID)
	if err != nil || operation.Status != "pending" || operation.Permission != "read" {
		t.Fatalf("an invalid answer consumed the question: %+v %v", operation, err)
	}
	operation, err = runtime.Executor.Decide(ctx, PRINCIPAL, result.OperationID, true, `{"answers":[{"selected":["24h"]},{"text":"the test key"}]}`)
	if err != nil || operation.Status != "success" {
		t.Fatalf("answer: %+v %v", operation, err)
	}
	var output AskQuestionOutput
	if err := json.Unmarshal(operation.Result.Data, &output); err != nil || len(output.Answers) != 2 || output.Answers[0].Selected[0] != "24h" || output.Answers[1].Text != "the test key" || output.Answers[0].Question != "Which window?" {
		t.Fatalf("output: %s %v", operation.Result.Data, err)
	}

	skipped, err := runtime.Executor.Invoke(ctx, PRINCIPAL, ASK_QUESTION, json.RawMessage(question), "")
	if err != nil {
		t.Fatal(err)
	}
	operation, err = runtime.Executor.Decide(ctx, PRINCIPAL, skipped.OperationID, false, "")
	if err != nil || operation.Result.Status != "rejected" {
		t.Fatalf("skip: %+v %v", operation, err)
	}
}
