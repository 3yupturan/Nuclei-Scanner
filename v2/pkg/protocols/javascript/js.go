package javascript

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alecthomas/chroma/quick"
	"github.com/ditashi/jsbeautifier-go/jsbeautifier"
	"github.com/dop251/goja"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/nuclei/v2/pkg/js/compiler"
	"github.com/projectdiscovery/nuclei/v2/pkg/js/gojs"
	"github.com/projectdiscovery/nuclei/v2/pkg/model"
	"github.com/projectdiscovery/nuclei/v2/pkg/operators"
	"github.com/projectdiscovery/nuclei/v2/pkg/operators/extractors"
	"github.com/projectdiscovery/nuclei/v2/pkg/operators/matchers"
	"github.com/projectdiscovery/nuclei/v2/pkg/output"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols/common/contextargs"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols/common/expressions"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols/common/generators"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols/common/helpers/eventcreator"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols/common/interactsh"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols/common/utils/vardump"
	protocolutils "github.com/projectdiscovery/nuclei/v2/pkg/protocols/utils"
	templateTypes "github.com/projectdiscovery/nuclei/v2/pkg/templates/types"
	"github.com/projectdiscovery/nuclei/v2/pkg/types"
	errorutil "github.com/projectdiscovery/utils/errors"
	urlutil "github.com/projectdiscovery/utils/url"
	"github.com/remeh/sizedwaitgroup"
)

// Request is a request for the javascript protocol
type Request struct {
	// Operators for the current request go here.
	operators.Operators `yaml:",inline,omitempty" json:",inline,omitempty"`
	CompiledOperators   *operators.Operators `yaml:"-" json:"-"`

	// description: |
	// ID is request id in that protocol
	ID string `yaml:"id,omitempty" json:"id,omitempty" jsonschema:"title=id of the request,description=ID is the optional ID of the Request"`

	// description: |
	//  Init is javascript code to execute after compiling template and before executing it on any target
	//  This is helpful for preparing payloads or other setup that maybe required for exploits
	Init string `yaml:"init,omitempty" json:"init,omitempty" jsonschema:"title=init javascript code,description=Init is the javascript code to execute after compiling template"`

	// description: |
	//   PreCondition is a condition which is evaluated before sending the request.
	PreCondition string `yaml:"pre-condition,omitempty" json:"pre-condition,omitempty" jsonschema:"title=pre-condition for the request,description=PreCondition is a condition which is evaluated before sending the request"`

	// description: |
	//   Args contains the arguments to pass to the javascript code.
	Args map[string]interface{} `yaml:"args,omitempty" json:"args,omitempty"`
	// description: |
	//   Code contains code to execute for the javascript request.
	Code string `yaml:"code,omitempty" json:"code,omitempty" jsonschema:"title=code to execute in javascript,description=Executes inline javascript code for the request"`

	// description: |
	//   StopAtFirstMatch stops processing the request at first match.
	StopAtFirstMatch bool `yaml:"stop-at-first-match,omitempty" json:"stop-at-first-match,omitempty" jsonschema:"title=stop at first match,description=Stop the execution after a match is found"`
	// description: |
	//   Attack is the type of payload combinations to perform.
	//
	//   Sniper is each payload once, pitchfork combines multiple payload sets and clusterbomb generates
	//   permutations and combinations for all payloads.
	AttackType generators.AttackTypeHolder `yaml:"attack,omitempty" json:"attack,omitempty" jsonschema:"title=attack is the payload combination,description=Attack is the type of payload combinations to perform,enum=sniper,enum=pitchfork,enum=clusterbomb"`
	// description: |
	//   Payload concurreny i.e threads for sending requests.
	// examples:
	//   - name: Send requests using 10 concurrent threads
	//     value: 10
	Threads int `yaml:"threads,omitempty" json:"threads,omitempty" jsonschema:"title=threads for sending requests,description=Threads specifies number of threads to use sending requests. This enables Connection Pooling"`
	// description: |
	//   Payloads contains any payloads for the current request.
	//
	//   Payloads support both key-values combinations where a list
	//   of payloads is provided, or optionally a single file can also
	//   be provided as payload which will be read on run-time.
	Payloads map[string]interface{} `yaml:"payloads,omitempty" json:"payloads,omitempty" jsonschema:"title=payloads for the webosocket request,description=Payloads contains any payloads for the current request"`

	generator *generators.PayloadGenerator

	// cache any variables that may be needed for operation.
	options *protocols.ExecutorOptions `yaml:"-" json:"-"`
}

// Compile compiles the request generators preparing any requests possible.
func (request *Request) Compile(options *protocols.ExecutorOptions) error {
	request.options = options

	var err error
	if len(request.Payloads) > 0 {
		request.generator, err = generators.New(request.Payloads, request.AttackType.Value, request.options.TemplatePath, options.Catalog, options.Options.AttackType, options.Options)
		if err != nil {
			return errors.Wrap(err, "could not parse payloads")
		}
	}

	if len(request.Matchers) > 0 || len(request.Extractors) > 0 {
		compiled := &request.Operators
		compiled.ExcludeMatchers = options.ExcludeMatchers
		compiled.TemplateID = options.TemplateID
		for _, matcher := range compiled.Matchers {
			if matcher.Part == "" && matcher.Type.MatcherType != matchers.DSLMatcher {
				matcher.Part = "response"
			}
		}
		if err := compiled.Compile(); err != nil {
			return errorutil.NewWithTag(request.TemplateID, "could not compile operators got %v", err)
		}
		request.CompiledOperators = compiled
	}

	// "Port" is a special variable and it should not contains any dsl expressions
	if strings.Contains(request.getPort(), "{{") {
		return errorutil.NewWithTag(request.TemplateID, "'Port' variable cannot contain any dsl expressions")
	}

	if request.Init != "" {
		// execute init code if any
		if request.options.Options.Debug || request.options.Options.DebugRequests {
			gologger.Debug().Msgf("[%s] Executing Template Init\n", request.TemplateID)
			var highlightFormatter = "terminal256"
			if request.options.Options.NoColor {
				highlightFormatter = "text"
			}
			var buff bytes.Buffer
			_ = quick.Highlight(&buff, beautifyJavascript(request.Init), "javascript", highlightFormatter, "monokai")
			prettyPrint(request.TemplateID, buff.String())
		}

		opts := &compiler.ExecuteOptions{}
		// register 'export' function to export variables from init code
		// these are saved in args and are available in pre-condition and request code
		opts.Callback = func(runtime *goja.Runtime) error {
			err := gojs.RegisterFuncWithSignature(runtime, gojs.FuncOpts{
				Name: "set",
				Signatures: []string{
					"set(string, interface{})",
				},
				Description: "set variable from init code. this function is available in init code block only",
				FuncDecl: func(varname string, value any) error {
					if varname == "" {
						return fmt.Errorf("variable name cannot be empty")
					}
					if value == nil {
						return fmt.Errorf("variable value cannot be empty")
					}
					if request.Args == nil {
						request.Args = make(map[string]interface{})
					}
					request.Args[varname] = value
					return nil
				},
			})
			if err != nil {
				return err
			}

			return gojs.RegisterFuncWithSignature(runtime, gojs.FuncOpts{
				Name: "updatePayload",
				Signatures: []string{
					"updatePayload(string, interface{})",
				},
				Description: "update/override any payload from init code. this function is available in init code block only",
				FuncDecl: func(varname string, Value any) error {
					if request.Payloads == nil {
						request.Payloads = make(map[string]interface{})
					}
					if request.generator != nil {
						request.Payloads[varname] = Value
						request.generator, err = generators.New(request.Payloads, request.AttackType.Value, request.options.TemplatePath, options.Catalog, options.Options.AttackType, options.Options)
						if err != nil {
							return err
						}
					} else {
						return fmt.Errorf("payloads not defined and cannot be updated")
					}
					return nil
				},
			})
		}

		args := compiler.NewExecuteArgs()
		allVars := generators.MergeMaps(options.Variables.GetAll(), options.Options.Vars.AsMap(), request.options.Constants)
		// proceed with whatever args we have
		args.Args, _ = request.evaluateArgs(allVars, options, true)

		result, err := request.options.JsCompiler.ExecuteWithOptions(request.Init, args, opts)
		if err != nil {
			return errorutil.NewWithTag(request.TemplateID, "could not execute pre-condition: %s", err)
		}
		if types.ToString(result["error"]) != "" {
			gologger.Warning().Msgf("[%s] Init failed with error %v\n", request.TemplateID, result["error"])
			return nil
		} else {
			if request.options.Options.Debug || request.options.Options.DebugResponse {
				gologger.Debug().Msgf("[%s] Init executed successfully\n", request.TemplateID)
				gologger.Debug().Msgf("[%s] Init result: %v\n", request.TemplateID, result["response"])
			}
		}
	}

	return nil
}

// Options returns executer options for http request
func (r *Request) Options() *protocols.ExecutorOptions {
	return r.options
}

// Requests returns the total number of requests the rule will perform
func (request *Request) Requests() int {
	pre_conditions := 0
	if request.PreCondition != "" {
		pre_conditions = 1
	}
	if request.generator != nil {
		payloadRequests := request.generator.NewIterator().Total()
		return payloadRequests + pre_conditions
	}
	return 1 + pre_conditions
}

// GetID returns the ID for the request if any.
func (request *Request) GetID() string {
	return request.ID
}

// ExecuteWithResults executes the protocol requests and returns results instead of writing them.
func (request *Request) ExecuteWithResults(target *contextargs.Context, dynamicValues, previous output.InternalEvent, callback protocols.OutputEventCallback) error {

	input := target.Clone()
	// use network port updates input with new port requested in template file
	// and it is ignored if input port is not standard http(s) ports like 80,8080,8081 etc
	// idea is to reduce redundant dials to http ports
	if err := input.UseNetworkPort(request.getPort(), request.getExcludePorts()); err != nil {
		gologger.Debug().Msgf("Could not network port from constants: %s\n", err)
	}

	hostPort, err := getAddress(input.MetaInput.Input)
	if err != nil {
		request.options.Progress.IncrementFailedRequestsBy(1)
		return err
	}
	hostname, port, _ := net.SplitHostPort(hostPort)
	if hostname == "" {
		hostname = hostPort
	}

	requestOptions := request.options
	templateCtx := request.options.GetTemplateCtx(input.MetaInput)

	payloadValues := generators.BuildPayloadFromOptions(request.options.Options)
	for k, v := range dynamicValues {
		payloadValues[k] = v
	}

	payloadValues["Hostname"] = hostPort
	payloadValues["Host"] = hostname
	payloadValues["Port"] = port

	hostnameVariables := protocolutils.GenerateDNSVariables(hostname)
	values := generators.MergeMaps(payloadValues, hostnameVariables, request.options.Constants, templateCtx.GetAll())
	variablesMap := request.options.Variables.Evaluate(values)
	payloadValues = generators.MergeMaps(variablesMap, payloadValues, request.options.Constants, hostnameVariables)
	// export all variables to template context
	templateCtx.Merge(payloadValues)

	if vardump.EnableVarDump {
		gologger.Debug().Msgf("Protocol request variables: \n%s\n", vardump.DumpVariables(payloadValues))
	}

	if request.PreCondition != "" {
		payloads := generators.MergeMaps(payloadValues, previous)

		if request.options.Options.Debug || request.options.Options.DebugRequests {
			gologger.Debug().Msgf("[%s] Executing Precondition for request\n", request.TemplateID)
			var highlightFormatter = "terminal256"
			if requestOptions.Options.NoColor {
				highlightFormatter = "text"
			}
			var buff bytes.Buffer
			_ = quick.Highlight(&buff, beautifyJavascript(request.PreCondition), "javascript", highlightFormatter, "monokai")
			prettyPrint(request.TemplateID, buff.String())
		}

		argsCopy, err := request.getArgsCopy(input, payloads, requestOptions, true)
		if err != nil {
			return err
		}
		argsCopy.TemplateCtx = templateCtx.GetAll()

		result, err := request.options.JsCompiler.ExecuteWithOptions(request.PreCondition, argsCopy, nil)
		if err != nil {
			return errorutil.NewWithTag(request.TemplateID, "could not execute pre-condition: %s", err)
		}
		if !result.GetSuccess() || types.ToString(result["error"]) != "" {
			gologger.Warning().Msgf("[%s] Precondition for request %s was not satisfied\n", request.TemplateID, request.PreCondition)
			request.options.Progress.IncrementFailedRequestsBy(1)
			return nil
		}
		if request.options.Options.Debug || request.options.Options.DebugRequests {
			request.options.Progress.IncrementRequests()
			gologger.Debug().Msgf("[%s] Precondition for request was satisfied\n", request.TemplateID)
		}
	}

	if request.generator != nil && request.Threads > 1 {
		request.executeRequestParallel(context.Background(), hostPort, hostname, input, payloadValues, callback)
		return nil
	}

	var gotMatches bool
	if request.generator != nil {
		iterator := request.generator.NewIterator()

		for {
			value, ok := iterator.Value()
			if !ok {
				return nil
			}

			if err := request.executeRequestWithPayloads(hostPort, input, hostname, value, payloadValues, func(result *output.InternalWrappedEvent) {
				if result.OperatorsResult != nil && result.OperatorsResult.Matched {
					gotMatches = true
					request.options.Progress.IncrementMatched()
				}
				callback(result)
			}, requestOptions); err != nil {
				_ = err
				// Review: should we log error here?
				// it is technically not error as it is expected to fail
				// gologger.Warning().Msgf("Could not execute request: %s\n", err)
				// do not return even if error occured
			}
			// If this was a match, and we want to stop at first match, skip all further requests.
			shouldStopAtFirstMatch := request.options.Options.StopAtFirstMatch || request.StopAtFirstMatch
			if shouldStopAtFirstMatch && gotMatches {
				return nil
			}
		}
	}
	return request.executeRequestWithPayloads(hostPort, input, hostname, nil, payloadValues, callback, requestOptions)
}

func (request *Request) executeRequestParallel(ctxParent context.Context, hostPort, hostname string, input *contextargs.Context, payloadValues map[string]interface{}, callback protocols.OutputEventCallback) {
	threads := request.Threads
	if threads == 0 {
		threads = 1
	}
	ctx, cancel := context.WithCancel(ctxParent)
	defer cancel()
	requestOptions := request.options
	gotmatches := &atomic.Bool{}

	sg := sizedwaitgroup.New(threads)
	if request.generator != nil {
		iterator := request.generator.NewIterator()
		for {
			value, ok := iterator.Value()
			if !ok {
				break
			}
			sg.Add()
			go func() {
				defer sg.Done()
				if ctx.Err() != nil {
					// work already done exit
					return
				}
				shouldStopAtFirstMatch := request.options.Options.StopAtFirstMatch || request.StopAtFirstMatch
				if err := request.executeRequestWithPayloads(hostPort, input, hostname, value, payloadValues, func(result *output.InternalWrappedEvent) {
					if result.OperatorsResult != nil && result.OperatorsResult.Matched {
						fmt.Print("got match\n")
						gotmatches.Store(true)
					}
					callback(result)
				}, requestOptions); err != nil {
					_ = err
					// Review: should we log error here?
					// it is technically not error as it is expected to fail
					// gologger.Warning().Msgf("Could not execute request: %s\n", err)
					// do not return even if error occured
				}
				// If this was a match, and we want to stop at first match, skip all further requests.

				if shouldStopAtFirstMatch && gotmatches.Load() {
					fmt.Printf("canceling\n")
					cancel()
					return
				}
			}()
		}
	}
	sg.Wait()
	if gotmatches.Load() {
		request.options.Progress.IncrementMatched()
	}
}

func (request *Request) executeRequestWithPayloads(hostPort string, input *contextargs.Context, hostname string, payload map[string]interface{}, previous output.InternalEvent, callback protocols.OutputEventCallback, requestOptions *protocols.ExecutorOptions) error {
	payloadValues := generators.MergeMaps(payload, previous)
	argsCopy, err := request.getArgsCopy(input, payloadValues, requestOptions, false)
	if err != nil {
		return err
	}
	argsCopy.TemplateCtx = request.options.GetTemplateCtx(input.MetaInput).GetAll()

	var requestData = []byte(request.Code)
	var interactshURLs []string
	if request.options.Interactsh != nil {
		var transformedData string
		transformedData, interactshURLs = request.options.Interactsh.Replace(string(request.Code), []string{})
		requestData = []byte(transformedData)
	}

	results, err := request.options.JsCompiler.ExecuteWithOptions(string(requestData), argsCopy, &compiler.ExecuteOptions{
		Pool: false,
	})
	if err != nil {
		// shouldn't fail even if it returned error instead create a failure event
		results = compiler.ExecuteResult{"success": false, "error": err.Error()}
	}
	request.options.Progress.IncrementRequests()

	requestOptions.Output.Request(requestOptions.TemplateID, hostPort, request.Type().String(), err)
	gologger.Verbose().Msgf("[%s] Sent Javascript request to %s", request.TemplateID, hostPort)

	if requestOptions.Options.Debug || requestOptions.Options.DebugRequests || requestOptions.Options.StoreResponse {
		msg := fmt.Sprintf("[%s] Dumped Javascript request for %s:\nVariables:\n %v", requestOptions.TemplateID, input.MetaInput.Input, vardump.DumpVariables(argsCopy.Args))

		if requestOptions.Options.Debug || requestOptions.Options.DebugRequests {
			gologger.Debug().Str("address", input.MetaInput.Input).Msg(msg)
			var highlightFormatter = "terminal256"
			if requestOptions.Options.NoColor {
				highlightFormatter = "text"
			}
			var buff bytes.Buffer
			_ = quick.Highlight(&buff, beautifyJavascript(request.Code), "javascript", highlightFormatter, "monokai")
			prettyPrint(request.TemplateID, buff.String())
		}
		if requestOptions.Options.StoreResponse {
			request.options.Output.WriteStoreDebugData(input.MetaInput.Input, request.options.TemplateID, request.Type().String(), msg)
		}
	}

	data := make(map[string]interface{})
	for k, v := range payloadValues {
		data[k] = v
	}
	data["type"] = request.Type().String()
	for k, v := range results {
		data[k] = v
	}
	data["request"] = beautifyJavascript(request.Code)
	data["host"] = input.MetaInput.Input
	data["matched"] = hostPort
	data["template-path"] = requestOptions.TemplatePath
	data["template-id"] = requestOptions.TemplateID
	data["template-info"] = requestOptions.TemplateInfo
	if request.StopAtFirstMatch || request.options.StopAtFirstMatch {
		data["stop-at-first-match"] = true
	}

	// add and get values from templatectx
	request.options.AddTemplateVars(input.MetaInput, request.Type(), request.GetID(), data)
	data = generators.MergeMaps(data, request.options.GetTemplateCtx(input.MetaInput).GetAll())

	if requestOptions.Options.Debug || requestOptions.Options.DebugRequests || requestOptions.Options.StoreResponse {
		msg := fmt.Sprintf("[%s] Dumped Javascript response for %s:\n%v", requestOptions.TemplateID, input.MetaInput.Input, vardump.DumpVariables(results))
		if requestOptions.Options.Debug || requestOptions.Options.DebugRequests {
			gologger.Debug().Str("address", input.MetaInput.Input).Msg(msg)
		}
		if requestOptions.Options.StoreResponse {
			request.options.Output.WriteStoreDebugData(input.MetaInput.Input, request.options.TemplateID, request.Type().String(), msg)
		}
	}

	if _, ok := data["error"]; ok {
		event := eventcreator.CreateEventWithAdditionalOptions(request, generators.MergeMaps(data, payloadValues), request.options.Options.Debug || request.options.Options.DebugResponse, func(wrappedEvent *output.InternalWrappedEvent) {
			wrappedEvent.OperatorsResult.PayloadValues = payload
		})
		callback(event)
		return err
	}

	if request.options.Interactsh != nil {
		request.options.Interactsh.MakePlaceholders(interactshURLs, data)
	}

	var event *output.InternalWrappedEvent
	if len(interactshURLs) == 0 {
		event = eventcreator.CreateEventWithAdditionalOptions(request, generators.MergeMaps(data, payloadValues), request.options.Options.Debug || request.options.Options.DebugResponse, func(wrappedEvent *output.InternalWrappedEvent) {
			wrappedEvent.OperatorsResult.PayloadValues = payload
		})
		callback(event)
	} else if request.options.Interactsh != nil {
		event = &output.InternalWrappedEvent{InternalEvent: data, UsesInteractsh: true}
		request.options.Interactsh.RequestEvent(interactshURLs, &interactsh.RequestData{
			MakeResultFunc: request.MakeResultEvent,
			Event:          event,
			Operators:      request.CompiledOperators,
			MatchFunc:      request.Match,
			ExtractFunc:    request.Extract,
		})
	}
	return nil
}

func (request *Request) getArgsCopy(input *contextargs.Context, payloadValues map[string]interface{}, requestOptions *protocols.ExecutorOptions, ignoreErrors bool) (*compiler.ExecuteArgs, error) {
	// Template args from payloads
	argsCopy, err := request.evaluateArgs(payloadValues, requestOptions, ignoreErrors)
	if err != nil {
		requestOptions.Output.Request(requestOptions.TemplateID, input.MetaInput.Input, request.Type().String(), err)
		requestOptions.Progress.IncrementFailedRequestsBy(1)
	}
	// "Port" is a special variable that is considered as network port
	// and is conditional based on input port and default port specified in input
	argsCopy["Port"] = input.Port()

	return &compiler.ExecuteArgs{Args: argsCopy}, nil
}

// evaluateArgs evaluates arguments using available payload values and returns a copy of args
func (request *Request) evaluateArgs(payloadValues map[string]interface{}, requestOptions *protocols.ExecutorOptions, ignoreErrors bool) (map[string]interface{}, error) {
	argsCopy := make(map[string]interface{})
mainLoop:
	for k, v := range request.Args {
		if vVal, ok := v.(string); ok && strings.Contains(vVal, "{") {
			finalAddress, dataErr := expressions.Evaluate(vVal, payloadValues)
			if dataErr != nil {
				return nil, errors.Wrap(dataErr, "could not evaluate template expressions")
			}
			if finalAddress == vVal && ignoreErrors {
				argsCopy[k] = ""
				continue mainLoop
			}
			argsCopy[k] = finalAddress
		} else {
			argsCopy[k] = v
		}
	}
	return argsCopy, nil
}

// RequestPartDefinitions contains a mapping of request part definitions and their
// description. Multiple definitions are separated by commas.
// Definitions not having a name (generated on runtime) are prefixed & suffixed by <>.
var RequestPartDefinitions = map[string]string{
	"type":     "Type is the type of request made",
	"response": "Javascript protocol result response",
	"host":     "Host is the input to the template",
	"matched":  "Matched is the input which was matched upon",
}

// getAddress returns the address of the host to make request to
func getAddress(toTest string) (string, error) {
	urlx, err := urlutil.Parse(toTest)
	if err != nil {
		// use given input instead of url parsing failure
		return toTest, nil
	}
	return urlx.Host, nil
}

// Match performs matching operation for a matcher on model and returns:
// true and a list of matched snippets if the matcher type is supports it
// otherwise false and an empty string slice
func (request *Request) Match(data map[string]interface{}, matcher *matchers.Matcher) (bool, []string) {
	return protocols.MakeDefaultMatchFunc(data, matcher)
}

// Extract performs extracting operation for an extractor on model and returns true or false.
func (request *Request) Extract(data map[string]interface{}, matcher *extractors.Extractor) map[string]struct{} {
	return protocols.MakeDefaultExtractFunc(data, matcher)
}

// MakeResultEvent creates a result event from internal wrapped event
func (request *Request) MakeResultEvent(wrapped *output.InternalWrappedEvent) []*output.ResultEvent {
	return protocols.MakeDefaultResultEvent(request, wrapped)
}

// GetCompiledOperators returns a list of the compiled operators
func (request *Request) GetCompiledOperators() []*operators.Operators {
	return []*operators.Operators{request.CompiledOperators}
}

// Type returns the type of the protocol request
func (request *Request) Type() templateTypes.ProtocolType {
	return templateTypes.JavascriptProtocol
}

func (request *Request) getPort() string {
	for k, v := range request.Args {
		if strings.EqualFold(k, "Port") {
			return types.ToString(v)
		}
	}
	return ""
}

func (request *Request) getExcludePorts() string {
	for k, v := range request.Args {
		if strings.EqualFold(k, "exclude-ports") {
			return types.ToString(v)
		}
	}
	return ""
}

func (request *Request) MakeResultEventItem(wrapped *output.InternalWrappedEvent) *output.ResultEvent {
	data := &output.ResultEvent{
		TemplateID:       types.ToString(wrapped.InternalEvent["template-id"]),
		TemplatePath:     types.ToString(wrapped.InternalEvent["template-path"]),
		Info:             wrapped.InternalEvent["template-info"].(model.Info),
		Type:             types.ToString(wrapped.InternalEvent["type"]),
		Host:             types.ToString(wrapped.InternalEvent["host"]),
		Matched:          types.ToString(wrapped.InternalEvent["matched"]),
		Metadata:         wrapped.OperatorsResult.PayloadValues,
		ExtractedResults: wrapped.OperatorsResult.OutputExtracts,
		Timestamp:        time.Now(),
		MatcherStatus:    true,
		Request:          types.ToString(wrapped.InternalEvent["request"]),
		Response:         types.ToString(wrapped.InternalEvent["response"]),
		IP:               types.ToString(wrapped.InternalEvent["ip"]),
	}
	return data
}

func beautifyJavascript(code string) string {
	opts := jsbeautifier.DefaultOptions()
	beautified, err := jsbeautifier.Beautify(&code, opts)
	if err != nil {
		return code
	}
	return beautified
}

func prettyPrint(templateId string, buff string) {
	lines := strings.Split(buff, "\n")
	final := []string{}
	for _, v := range lines {
		if v != "" {
			final = append(final, "\t"+v)
		}
	}
	gologger.Debug().Msgf(" [%v] Javascript Code:\n\n%v\n\n", templateId, strings.Join(final, "\n"))
}
