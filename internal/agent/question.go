package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/oh-my-cpa/oh-my-cpa/internal/capability"
)

// ASK_QUESTION is the one capability that belongs to the conversation rather than to OMC: the
// model asks the operator something and the turn waits for the reply.
//
// It is modelled on the question tools of established coding agents - a few questions at once,
// each with a short list of labelled options, single or multiple choice, and a typed answer always
// available - because a model asking one open question per round costs the operator a round trip
// per detail, while a model offering only fixed options traps them when none fits.
const ASK_QUESTION = "ask_question"

const (
	MAX_QUESTIONS           = 4
	MAX_QUESTION_OPTIONS    = 6
	MAX_QUESTION_CHARS      = 500
	MAX_QUESTION_HEADER     = 24
	MAX_OPTION_LABEL_CHARS  = 80
	MAX_OPTION_DETAIL_CHARS = 240
	MAX_TYPED_ANSWER_CHARS  = 2000

	ASK_QUESTION_DESCRIPTION = "Ask the operator 1-4 questions and wait for the reply. Use it when a request is ambiguous or a choice depends on the operator's preference, instead of guessing. Offer 2-6 short options per question when the likely answers are known; the operator can always type their own answer instead. Never use it to ask for approval of a change (writes already ask) or for secrets. A `rejected` result means the operator skipped the questions."
)

type QuestionOption struct {
	Label       string `json:"label" jsonschema:"The choice as the operator reads it, at most 80 characters"`
	Description string `json:"description,omitempty" jsonschema:"Optional consequence of this choice, at most 240 characters"`
}

type Question struct {
	Question      string           `json:"question" jsonschema:"The complete question, ending with a question mark"`
	Header        string           `json:"header,omitempty" jsonschema:"Optional short label shown above the question, at most 24 characters"`
	Options       []QuestionOption `json:"options,omitempty" jsonschema:"0 or 2-6 options; omit for a free-text question"`
	IsMultiSelect bool             `json:"multi_select,omitempty" jsonschema:"Allow more than one option to be chosen"`
}

type AskQuestionInput struct {
	Questions []Question `json:"questions" jsonschema:"1-4 questions asked together"`
}

type QuestionAnswer struct {
	Question string   `json:"question"`
	Selected []string `json:"selected"`
	Text     string   `json:"text,omitempty"`
}

type AskQuestionOutput struct {
	Answers []QuestionAnswer `json:"answers"`
}

// operatorAnswer is what the browser posts: one entry per question, in the order asked.
type operatorAnswer struct {
	Answers []struct {
		Selected []string `json:"selected"`
		Text     string   `json:"text"`
	} `json:"answers"`
}

// RegisterAskQuestion adds the question capability. It is offered to the built-in Agent only: an
// external MCP client has its own way to ask its user, and could not wait on this console's.
func RegisterAskQuestion(registry *capability.Registry) error {
	metadata := capability.Metadata{Name: ASK_QUESTION, Description: ASK_QUESTION_DESCRIPTION, Version: 1, Permission: "read", Risk: "low", HumanInput: "answer", Adapters: []string{"agent"}}
	return capability.Register(registry, metadata, func(_ context.Context, input AskQuestionInput) (capability.Preview, error) {
		if err := validateQuestions(input.Questions); err != nil {
			return capability.Preview{}, err
		}
		return capability.Preview{Target: input.Questions[0].Question, Changes: input}, nil
	}, func(_ context.Context, input AskQuestionInput, _, answer string) (AskQuestionOutput, error) {
		return answerQuestions(input.Questions, answer)
	})
}

func validateQuestions(questions []Question) error {
	if len(questions) == 0 || len(questions) > MAX_QUESTIONS {
		return errors.New("invalid_tool_arguments")
	}
	for _, question := range questions {
		if strings.TrimSpace(question.Question) == "" || utf8.RuneCountInString(question.Question) > MAX_QUESTION_CHARS || utf8.RuneCountInString(question.Header) > MAX_QUESTION_HEADER {
			return errors.New("invalid_tool_arguments")
		}
		if len(question.Options) == 1 || len(question.Options) > MAX_QUESTION_OPTIONS || question.IsMultiSelect && len(question.Options) == 0 {
			return errors.New("invalid_tool_arguments")
		}
		labels := map[string]bool{}
		for _, option := range question.Options {
			label := strings.TrimSpace(option.Label)
			if label == "" || labels[label] || utf8.RuneCountInString(option.Label) > MAX_OPTION_LABEL_CHARS || utf8.RuneCountInString(option.Description) > MAX_OPTION_DETAIL_CHARS {
				return errors.New("invalid_tool_arguments")
			}
			labels[label] = true
		}
	}
	return nil
}

// answerQuestions checks the operator's reply against the questions it answers: one entry per
// question, choices drawn from that question's own options, and something - a choice or typed
// text - for every question.
func answerQuestions(questions []Question, raw string) (AskQuestionOutput, error) {
	var answer operatorAnswer
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&answer) != nil || len(answer.Answers) != len(questions) {
		return AskQuestionOutput{}, errors.New("invalid_answer")
	}
	output := AskQuestionOutput{Answers: make([]QuestionAnswer, 0, len(questions))}
	for index, question := range questions {
		reply := answer.Answers[index]
		text := strings.TrimSpace(reply.Text)
		if utf8.RuneCountInString(text) > MAX_TYPED_ANSWER_CHARS || len(reply.Selected) > len(question.Options) || !question.IsMultiSelect && len(reply.Selected) > 1 {
			return AskQuestionOutput{}, errors.New("invalid_answer")
		}
		selected := []string{}
		for _, label := range reply.Selected {
			if !hasOption(question.Options, label) || slices.Contains(selected, label) {
				return AskQuestionOutput{}, errors.New("invalid_answer")
			}
			selected = append(selected, label)
		}
		if len(selected) == 0 && text == "" {
			return AskQuestionOutput{}, errors.New("invalid_answer")
		}
		output.Answers = append(output.Answers, QuestionAnswer{Question: question.Question, Selected: selected, Text: text})
	}
	return output, nil
}

func hasOption(options []QuestionOption, label string) bool {
	for _, option := range options {
		if strings.TrimSpace(option.Label) == label {
			return true
		}
	}
	return false
}
