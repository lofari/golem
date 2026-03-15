package runner

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Step types
const (
	StepTypeAgentic = "agentic"
	StepTypeBuiltin = "builtin"
	StepTypeShell   = "shell"
)

// Control flow node types
const (
	ControlWhile = "while"
	ControlWhen  = "when"
	ControlIf    = "if"
)

// Reserved engine-managed keys
var reservedKeys = map[string]bool{
	"code":   true,
	"branch": true,
	"base":   true,
}

// Blueprint represents a parsed blueprint YAML file.
type Blueprint struct {
	Name            string                    `yaml:"name"`
	Description     string                    `yaml:"description"`
	InitialState    []string                  `yaml:"initial-state"`
	Config          map[string]any            `yaml:"config"`
	Steps           []Step                    `yaml:"-"`
	Errors          ErrorHandlers             `yaml:"errors"`
	Predicates      map[string]string         `yaml:"predicates"`
	parsedPredicates map[string]*PredicateExpr // unexported, cached at parse time
	pipeline        *Pipeline
}

// Step represents a single step in a blueprint pipeline.
type Step struct {
	Name          string      `yaml:"name"`
	Type          string      `yaml:"type"`
	Reads         []string    `yaml:"reads"`
	Writes        []string    `yaml:"writes"`
	OptionalReads []string    `yaml:"optional-reads"`
	Tools         []string    `yaml:"tools"`
	Prompt        string      `yaml:"prompt"`
	MaxTurns      int         `yaml:"max-turns"`
	Timeout       string      `yaml:"timeout"`
	Model         string      `yaml:"model"`
	Command       string      `yaml:"command"`
	StepErrors    *StepErrors `yaml:"errors"`
}

// StepErrors holds per-step error handling configuration.
type StepErrors struct {
	NonZero           string        `yaml:"non-zero"`
	Transient         *ErrorHandler `yaml:"transient"`
	MalformedOutput   *ErrorHandler `yaml:"malformed-output"`
	ContractViolation *ErrorHandler `yaml:"contract-violation"`
}

// ControlFlowNode represents a control flow construct (while, when, if).
type ControlFlowNode struct {
	Type        string
	Predicate   string
	Max         int
	StepRefs    []string
	ThenRefs    []string
	ElseRefs    []string
	InlineSteps []Step
	SubNodes    []PipelineNode // nested control flow + steps (used by when/while/if)
	ThenNodes   []PipelineNode // for if-then
	ElseNodes   []PipelineNode // for if-else
}

// PipelineNode is either a step or a control flow node.
type PipelineNode struct {
	Step        *Step
	ControlFlow *ControlFlowNode
}

// Pipeline is the ordered list of nodes with a lookup map.
type Pipeline struct {
	Nodes    []PipelineNode
	StepDefs map[string]*Step
}

// ErrorHandlers holds top-level error handling configuration.
type ErrorHandlers struct {
	Transient         ErrorHandler `yaml:"transient"`
	MalformedOutput   ErrorHandler `yaml:"malformed-output"`
	ContractViolation ErrorHandler `yaml:"contract-violation"`
}

// ErrorHandler configures how a specific error class is handled.
type ErrorHandler struct {
	Action string `yaml:"action"`
	Max    int    `yaml:"max"`
	Hint   string `yaml:"hint"`
}

var stepDefaults = map[string]struct {
	MaxTurns int
	Timeout  time.Duration
}{
	"plan":      {MaxTurns: 50, Timeout: 20 * time.Minute},
	"implement": {MaxTurns: 200, Timeout: 30 * time.Minute},
	"review":    {MaxTurns: 50, Timeout: 20 * time.Minute},
	"reflect":   {MaxTurns: 30, Timeout: 10 * time.Minute},
	"research":  {MaxTurns: 75, Timeout: 20 * time.Minute},
}

var defaultStepMaxTurns = 75
var defaultStepTimeout = 20 * time.Minute

var defaultTools = map[string][]string{
	"plan":      {"semantic_search", "find_callers", "find_dependencies", "find_co_changed"},
	"implement": {"semantic_search", "find_callers", "find_dependencies", "find_dependents", "find_co_changed", "find_execution_failures", "lsp_definition", "lsp_references", "lsp_hover", "lsp_diagnostics"},
	"review":    {"semantic_search", "find_callers", "find_dependencies"},
	"reflect":   {"semantic_search"},
	"research":  {"semantic_search", "find_callers", "find_dependencies", "find_co_changed", "find_execution_failures", "get_runtime_trace"},
}

// knownStepFields maps common typos to the correct field name.
var knownStepFields = map[string]string{
	"tool":          "tools",
	"write":         "writes",
	"read":          "reads",
	"optional-read": "optional-reads",
}

// validStepFields is the set of valid fields inside a step definition.
var validStepFields = map[string]bool{
	"type":           true,
	"reads":          true,
	"writes":         true,
	"optional-reads": true,
	"tools":          true,
	"prompt":         true,
	"max-turns":      true,
	"timeout":        true,
	"model":          true,
	"command":        true,
	"errors":         true,
}

// validControlFlowFields is the set of valid fields inside a control flow node.
var validControlFlowFields = map[string]bool{
	"predicate": true,
	"max":       true,
	"steps":     true,
	"then":      true,
	"else":      true,
}

// sliceContains checks if a string slice contains a value.
func sliceContains(slice []string, val string) bool {
	return slices.Contains(slice, val)
}

func isControlFlow(name string) bool {
	return name == ControlWhile || name == ControlWhen || name == ControlIf
}

// AllSteps returns all steps in the pipeline, including those inside nested control flow nodes.
func (p *Pipeline) AllSteps() []Step {
	var steps []Step
	for _, node := range p.Nodes {
		collectSteps(node, &steps)
	}
	return steps
}

func collectSteps(node PipelineNode, steps *[]Step) {
	if node.Step != nil {
		*steps = append(*steps, *node.Step)
	}
	if node.ControlFlow != nil {
		for _, s := range node.ControlFlow.InlineSteps {
			*steps = append(*steps, s)
		}
		for _, sub := range node.ControlFlow.SubNodes {
			collectSteps(sub, steps)
		}
		for _, sub := range node.ControlFlow.ThenNodes {
			collectSteps(sub, steps)
		}
		for _, sub := range node.ControlFlow.ElseNodes {
			collectSteps(sub, steps)
		}
	}
}

// ParseBlueprint parses and validates a blueprint YAML document.
func ParseBlueprint(data []byte) (*Blueprint, error) {
	// First: parse into yaml.Node for field-level validation
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("blueprint: YAML syntax error: %w", err)
	}

	// Parse basic top-level fields via structured unmarshal
	var bp Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("blueprint: parse error: %w", err)
	}

	// Validate top-level fields
	if err := validateTopLevelFields(&doc); err != nil {
		return nil, err
	}

	// Parse steps from yaml.Node tree
	steps, pipeline, err := parseSteps(&doc)
	if err != nil {
		return nil, err
	}
	bp.Steps = steps
	bp.pipeline = pipeline

	// Validate error handler actions
	if err := validateErrorHandlers(&bp.Errors); err != nil {
		return nil, err
	}

	parsed, err := parsePredicates(bp.Predicates)
	if err != nil {
		return nil, err
	}
	bp.parsedPredicates = parsed

	return &bp, nil
}

func parsePredicates(preds map[string]string) (map[string]*PredicateExpr, error) {
	if len(preds) == 0 {
		return nil, nil
	}
	result := make(map[string]*PredicateExpr, len(preds))
	for name, expr := range preds {
		parsed, err := ParsePredicateExpr(expr)
		if err != nil {
			return nil, fmt.Errorf("blueprint: predicate %q: %w", name, err)
		}
		result[name] = parsed
	}
	return result, nil
}

// validTopLevelFields is the set of valid fields at the document root.
var validTopLevelFields = map[string]bool{
	"name":          true,
	"description":   true,
	"initial-state": true,
	"config":        true,
	"steps":         true,
	"errors":        true,
	"predicates":    true,
}

// knownTopLevelFields maps common typos to the correct field name.
var knownTopLevelFields = map[string]string{
	"intial-state": "initial-state",
	"inital-state": "initial-state",
	"step":         "steps",
	"error":        "errors",
	"configs":      "config",
	"desc":         "description",
}

func validateTopLevelFields(doc *yaml.Node) error {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	var errs []string
	for i := 0; i < len(root.Content)-1; i += 2 {
		field := root.Content[i].Value
		if validTopLevelFields[field] {
			continue
		}
		if suggestion, ok := knownTopLevelFields[field]; ok {
			errs = append(errs, fmt.Sprintf("unknown field %q (did you mean %q?)", field, suggestion))
		} else {
			errs = append(errs, fmt.Sprintf("unknown field %q", field))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("blueprint: %s", strings.Join(errs, "; "))
	}
	return nil
}

// validErrorActions is the set of valid error handler action values.
var validErrorActions = map[string]bool{
	"":      true, // empty means use default
	"retry": true,
	"re-run": true,
	"halt":  true,
}

func validateErrorHandlers(eh *ErrorHandlers) error {
	handlers := map[string]string{
		"transient":          eh.Transient.Action,
		"malformed-output":   eh.MalformedOutput.Action,
		"contract-violation": eh.ContractViolation.Action,
	}
	for name, action := range handlers {
		if !validErrorActions[action] {
			return fmt.Errorf("blueprint: errors.%s: invalid action %q (valid: retry, re-run, halt)", name, action)
		}
	}
	return nil
}

// parseSteps walks the yaml.Node tree to extract steps with strict field validation.
func parseSteps(doc *yaml.Node) ([]Step, *Pipeline, error) {
	// doc is a DocumentNode; its first child is the root mapping
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil, fmt.Errorf("blueprint: expected document node")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("blueprint: expected mapping at root")
	}

	// Find the "steps" key
	var stepsNode *yaml.Node
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "steps" {
			stepsNode = root.Content[i+1]
			break
		}
	}
	if stepsNode == nil {
		return nil, nil, fmt.Errorf("blueprint: missing 'steps' key")
	}
	if stepsNode.Kind != yaml.SequenceNode {
		return nil, nil, fmt.Errorf("blueprint: 'steps' must be a sequence")
	}

	var steps []Step
	pipeline := &Pipeline{
		StepDefs: make(map[string]*Step),
	}
	seen := make(map[string]bool)

	for _, item := range stepsNode.Content {
		if item.Kind != yaml.MappingNode {
			return nil, nil, fmt.Errorf("blueprint: each step must be a mapping (line %d)", item.Line)
		}
		if len(item.Content) < 2 {
			return nil, nil, fmt.Errorf("blueprint: empty step entry (line %d)", item.Line)
		}

		// Each step item is a single-key mapping: stepName -> config
		stepName := item.Content[0].Value
		stepBody := item.Content[1]

		// Check for duplicate step names (skip control flow keywords — they can repeat)
		if !isControlFlow(stepName) {
			if seen[stepName] {
				return nil, nil, fmt.Errorf("blueprint: duplicate step name %q", stepName)
			}
			seen[stepName] = true
		}

		if isControlFlow(stepName) {
			cf, err := parseControlFlowNode(stepName, stepBody, seen)
			if err != nil {
				return nil, nil, err
			}
			pipeline.Nodes = append(pipeline.Nodes, PipelineNode{ControlFlow: cf})
			// Add inline steps to the flat steps list and stepDefs
			for i := range cf.InlineSteps {
				steps = append(steps, cf.InlineSteps[i])
				s := cf.InlineSteps[i]
				pipeline.StepDefs[s.Name] = &s
			}
		} else {
			step, err := parseStepNode(stepName, stepBody)
			if err != nil {
				return nil, nil, err
			}
			steps = append(steps, *step)
			pipeline.StepDefs[step.Name] = step
			pipeline.Nodes = append(pipeline.Nodes, PipelineNode{Step: step})
		}
	}

	return steps, pipeline, nil
}

// parseStepNode parses a single step from its yaml.Node body, with strict field checking.
func parseStepNode(name string, body *yaml.Node) (*Step, error) {
	// Validate fields
	if err := validateStepFields(name, body); err != nil {
		return nil, err
	}

	// Unmarshal the body into a Step
	step := &Step{Name: name}
	if body.Kind == yaml.ScalarNode && body.Value == "" {
		// Step with no body (e.g., `- git-setup:` with null value)
		return step, nil
	}
	if body.Kind == yaml.MappingNode && len(body.Content) == 0 {
		return step, nil
	}
	if err := body.Decode(step); err != nil {
		return nil, fmt.Errorf("blueprint: error decoding step %q: %w", name, err)
	}
	step.Name = name
	return step, nil
}

// validateStepFields checks for unknown fields and suggests corrections.
func validateStepFields(name string, body *yaml.Node) error {
	if body.Kind != yaml.MappingNode {
		return nil // scalar or null body, nothing to validate
	}

	var errs []string
	for i := 0; i < len(body.Content)-1; i += 2 {
		field := body.Content[i].Value
		if validStepFields[field] {
			continue
		}
		if suggestion, ok := knownStepFields[field]; ok {
			errs = append(errs, fmt.Sprintf("step %q: unknown field %q (did you mean %q?)", name, field, suggestion))
		} else {
			errs = append(errs, fmt.Sprintf("step %q: unknown field %q", name, field))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("blueprint: %s", strings.Join(errs, "; "))
	}
	return nil
}

// parseControlFlowNode parses a while/when/if node from its yaml.Node body.
func parseControlFlowNode(cfType string, body *yaml.Node, seen map[string]bool) (*ControlFlowNode, error) {
	if body.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("blueprint: control flow %q must be a mapping", cfType)
	}

	// Validate fields
	for i := 0; i < len(body.Content)-1; i += 2 {
		field := body.Content[i].Value
		if !validControlFlowFields[field] {
			return nil, fmt.Errorf("blueprint: control flow %q: unknown field %q", cfType, field)
		}
	}

	cf := &ControlFlowNode{Type: cfType}

	// Extract fields from the mapping
	for i := 0; i < len(body.Content)-1; i += 2 {
		key := body.Content[i].Value
		val := body.Content[i+1]

		switch key {
		case "predicate":
			cf.Predicate = val.Value
		case "max":
			var maxVal int
			if err := val.Decode(&maxVal); err != nil {
				return nil, fmt.Errorf("blueprint: control flow %q: invalid max: %w", cfType, err)
			}
			cf.Max = maxVal
		case "steps":
			refs, inlineSteps, nodes, err := parseControlFlowSteps(val, seen)
			if err != nil {
				return nil, err
			}
			cf.StepRefs = refs
			cf.InlineSteps = append(cf.InlineSteps, inlineSteps...)
			cf.SubNodes = nodes
		case "then":
			refs, inlineSteps, nodes, err := parseControlFlowSteps(val, seen)
			if err != nil {
				return nil, err
			}
			cf.ThenRefs = refs
			cf.InlineSteps = append(cf.InlineSteps, inlineSteps...)
			cf.ThenNodes = nodes
		case "else":
			refs, inlineSteps, nodes, err := parseControlFlowSteps(val, seen)
			if err != nil {
				return nil, err
			}
			cf.ElseRefs = refs
			cf.InlineSteps = append(cf.InlineSteps, inlineSteps...)
			cf.ElseNodes = nodes
		}
	}

	return cf, nil
}

// parseControlFlowSteps parses the steps/then/else value which can be a sequence of
// string references, inline step definitions, or nested control flow nodes.
func parseControlFlowSteps(node *yaml.Node, seen map[string]bool) ([]string, []Step, []PipelineNode, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, nil, nil, fmt.Errorf("blueprint: control flow steps must be a sequence")
	}

	var refs []string
	var inlineSteps []Step
	var nodes []PipelineNode

	for _, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			// String reference to a step name
			refs = append(refs, item.Value)
			nodes = append(nodes, PipelineNode{}) // placeholder, resolved at exec time
		case yaml.MappingNode:
			// Inline step or nested control flow
			if len(item.Content) < 2 {
				return nil, nil, nil, fmt.Errorf("blueprint: empty inline step")
			}
			name := item.Content[0].Value
			body := item.Content[1]

			if isControlFlow(name) {
				// Nested control flow node
				cf, err := parseControlFlowNode(name, body, seen)
				if err != nil {
					return nil, nil, nil, err
				}
				nodes = append(nodes, PipelineNode{ControlFlow: cf})
				// Add inline steps from nested control flow for contract validation
				inlineSteps = append(inlineSteps, cf.InlineSteps...)
				refs = append(refs, "") // empty ref signals a control flow node
			} else {
				if seen[name] {
					return nil, nil, nil, fmt.Errorf("blueprint: duplicate step name %q", name)
				}
				seen[name] = true
				step, err := parseStepNode(name, body)
				if err != nil {
					return nil, nil, nil, err
				}
				inlineSteps = append(inlineSteps, *step)
				refs = append(refs, name)
				nodes = append(nodes, PipelineNode{Step: step})
			}
		default:
			return nil, nil, nil, fmt.Errorf("blueprint: unexpected node type in control flow steps")
		}
	}

	return refs, inlineSteps, nodes, nil
}

// ValidateContracts checks that every step's reads are satisfied by prior writes or initial state.
func (bp *Blueprint) ValidateContracts() error {
	available := make(map[string]bool)
	for _, key := range bp.InitialState {
		available[key] = true
	}
	// Implicit engine-managed keys
	available["branch"] = true
	available["base"] = true

	conditionalWrites := make(map[string]bool)

	for _, node := range bp.pipeline.Nodes {
		if node.Step != nil {
			if err := validateStepContracts(node.Step, available, conditionalWrites); err != nil {
				return err
			}
			for _, w := range node.Step.Writes {
				available[w] = true
			}
		}
		if node.ControlFlow != nil {
			if err := validateControlFlowContracts(node.ControlFlow, available, conditionalWrites); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateStepContracts checks a single step's reads against available keys.
func validateStepContracts(step *Step, available, conditionalWrites map[string]bool) error {
	for _, key := range step.Reads {
		if !available[key] {
			if conditionalWrites[key] {
				return fmt.Errorf("contract violation: step %q reads %q which is only conditionally written; use optional-reads instead", step.Name, key)
			}
			return fmt.Errorf("contract violation: step %q reads %q which is not produced by any prior step or initial-state", step.Name, key)
		}
		if conditionalWrites[key] {
			return fmt.Errorf("contract violation: step %q reads %q which is only conditionally written; use optional-reads instead", step.Name, key)
		}
	}
	for _, key := range step.OptionalReads {
		if !available[key] && !conditionalWrites[key] {
			// optional-reads for keys not yet written is fine — they'll be absent at runtime
		}
	}
	return nil
}

// validateControlFlowContracts validates steps inside a control flow node and marks their writes as conditional.
func validateControlFlowContracts(cf *ControlFlowNode, available, conditionalWrites map[string]bool) error {
	for i := range cf.InlineSteps {
		step := &cf.InlineSteps[i]
		if err := validateStepContracts(step, available, conditionalWrites); err != nil {
			return err
		}
		// Writes inside control flow are conditional
		for _, w := range step.Writes {
			conditionalWrites[w] = true
		}
	}
	return nil
}

// RenderStepPrompt renders a prompt template by replacing ${key} tokens with state values.
func RenderStepPrompt(tmpl string, reads, optionalReads []string, state, config map[string]any, agentName, runID string) (string, error) {
	result := tmpl

	// Replace reads keys (guaranteed present)
	for _, key := range reads {
		val, ok := state[key]
		if !ok {
			return "", fmt.Errorf("template error: reads key %q not in state", key)
		}
		jsonVal, _ := json.Marshal(val)
		result = strings.ReplaceAll(result, "${"+key+"}", string(jsonVal))
	}

	// Replace optional-reads (omit if absent)
	for _, key := range optionalReads {
		token := "${" + key + "}"
		val, ok := state[key]
		if ok {
			jsonVal, _ := json.Marshal(val)
			result = strings.ReplaceAll(result, token, string(jsonVal))
		} else {
			lines := strings.Split(result, "\n")
			var filtered []string
			for _, line := range lines {
				if strings.Contains(line, token) {
					if len(filtered) > 0 && strings.HasPrefix(strings.TrimSpace(filtered[len(filtered)-1]), "#") {
						filtered = filtered[:len(filtered)-1]
					}
					continue
				}
				filtered = append(filtered, line)
			}
			result = strings.Join(filtered, "\n")
		}
	}

	// Replace config vars
	if config != nil {
		for key, val := range config {
			jsonVal, _ := json.Marshal(val)
			result = strings.ReplaceAll(result, "${config."+key+"}", string(jsonVal))
		}
	}

	// Replace agent.name and run.id
	result = strings.ReplaceAll(result, "${agent.name}", agentName)
	result = strings.ReplaceAll(result, "${run.id}", runID)

	// Check for unresolved tokens
	var unresolved []string
	remaining := result
	for {
		idx := strings.Index(remaining, "${")
		if idx == -1 {
			break
		}
		end := strings.Index(remaining[idx:], "}")
		if end == -1 {
			break
		}
		unresolved = append(unresolved, remaining[idx:idx+end+1])
		remaining = remaining[idx+end+1:]
	}
	if len(unresolved) > 0 {
		return "", fmt.Errorf("template error: unresolved tokens %s (typo in template?)", strings.Join(unresolved, ", "))
	}

	return result, nil
}
