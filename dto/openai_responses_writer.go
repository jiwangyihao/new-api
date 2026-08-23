package dto

import (
	"encoding/json"
	"io"

	"github.com/QuantumNous/new-api/common"
)

// WriteJSON writes the same JSON bytes as encoding/json.Marshal without
// materializing the complete Responses payload in memory.
func (r OpenAIResponsesRequest) WriteJSON(writer io.Writer) error {
	if err := writeResponsesJSONString(writer, `{"model":`, r.Model); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"input":`, r.Input); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"include":`, r.Include); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"conversation":`, r.Conversation); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"context_management":`, r.ContextManagement); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"instructions":`, r.Instructions); err != nil {
		return err
	}
	if r.MaxOutputTokens != nil {
		if err := writeResponsesJSONField(writer, `,"max_output_tokens":`, r.MaxOutputTokens); err != nil {
			return err
		}
	}
	if r.TopLogProbs != nil {
		if err := writeResponsesJSONField(writer, `,"top_logprobs":`, r.TopLogProbs); err != nil {
			return err
		}
	}
	if err := writeResponsesRawField(writer, `,"metadata":`, r.Metadata); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"parallel_tool_calls":`, r.ParallelToolCalls); err != nil {
		return err
	}
	if r.PreviousResponseID != "" {
		if err := writeResponsesJSONString(writer, `,"previous_response_id":`, r.PreviousResponseID); err != nil {
			return err
		}
	}
	if r.Reasoning != nil {
		if err := writeResponsesJSONField(writer, `,"reasoning":`, r.Reasoning); err != nil {
			return err
		}
	}
	if r.ServiceTier != "" {
		if err := writeResponsesJSONString(writer, `,"service_tier":`, r.ServiceTier); err != nil {
			return err
		}
	}
	if err := writeResponsesRawField(writer, `,"store":`, r.Store); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"prompt_cache_key":`, r.PromptCacheKey); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"prompt_cache_retention":`, r.PromptCacheRetention); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"safety_identifier":`, r.SafetyIdentifier); err != nil {
		return err
	}
	if r.Stream != nil {
		if err := writeResponsesJSONField(writer, `,"stream":`, r.Stream); err != nil {
			return err
		}
	}
	if r.StreamOptions != nil {
		if err := writeResponsesJSONField(writer, `,"stream_options":`, r.StreamOptions); err != nil {
			return err
		}
	}
	if r.Temperature != nil {
		if err := writeResponsesJSONField(writer, `,"temperature":`, r.Temperature); err != nil {
			return err
		}
	}
	if err := writeResponsesRawField(writer, `,"text":`, r.Text); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"tool_choice":`, r.ToolChoice); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"tools":`, r.Tools); err != nil {
		return err
	}
	if r.TopP != nil {
		if err := writeResponsesJSONField(writer, `,"top_p":`, r.TopP); err != nil {
			return err
		}
	}
	if err := writeResponsesRawField(writer, `,"truncation":`, r.Truncation); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"user":`, r.User); err != nil {
		return err
	}
	if r.MaxToolCalls != nil {
		if err := writeResponsesJSONField(writer, `,"max_tool_calls":`, r.MaxToolCalls); err != nil {
			return err
		}
	}
	if err := writeResponsesRawField(writer, `,"prompt":`, r.Prompt); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"enable_thinking":`, r.EnableThinking); err != nil {
		return err
	}
	if err := writeResponsesRawField(writer, `,"preset":`, r.Preset); err != nil {
		return err
	}
	return writeResponsesString(writer, "}")
}

func writeResponsesJSONString(writer io.Writer, prefix string, value string) error {
	return writeResponsesJSONField(writer, prefix, value)
}

func writeResponsesJSONField(writer io.Writer, prefix string, value any) error {
	encoded, err := common.Marshal(value)
	if err != nil {
		return err
	}
	if err := writeResponsesString(writer, prefix); err != nil {
		return err
	}
	return writeResponsesBytes(writer, encoded)
}

func writeResponsesRawField(writer io.Writer, prefix string, value json.RawMessage) error {
	if len(value) == 0 {
		return nil
	}
	if !json.Valid(value) {
		_, err := common.Marshal(value)
		return err
	}
	if err := writeResponsesString(writer, prefix); err != nil {
		return err
	}
	return writeResponsesCompactRaw(writer, value)
}

func writeResponsesCompactRaw(writer io.Writer, value []byte) error {
	start := 0
	inString := false
	escaped := false
	flush := func(end int) error {
		if start >= end {
			return nil
		}
		if err := writeResponsesBytes(writer, value[start:end]); err != nil {
			return err
		}
		return nil
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if !inString {
			if char == '"' {
				inString = true
				continue
			}
			if char == ' ' || char == '\t' || char == '\r' || char == '\n' {
				if err := flush(index); err != nil {
					return err
				}
				start = index + 1
			}
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			inString = false
			continue
		}
		if char == '<' || char == '>' || char == '&' {
			if err := flush(index); err != nil {
				return err
			}
			var escapedHTML string
			switch char {
			case '<':
				escapedHTML = `\u003c`
			case '>':
				escapedHTML = `\u003e`
			default:
				escapedHTML = `\u0026`
			}
			if err := writeResponsesString(writer, escapedHTML); err != nil {
				return err
			}
			start = index + 1
			continue
		}
		if char == 0xE2 && index+2 < len(value) && value[index+1] == 0x80 && (value[index+2] == 0xA8 || value[index+2] == 0xA9) {
			if err := flush(index); err != nil {
				return err
			}
			if value[index+2] == 0xA8 {
				if err := writeResponsesString(writer, `\u2028`); err != nil {
					return err
				}
			} else if err := writeResponsesString(writer, `\u2029`); err != nil {
				return err
			}
			index += 2
			start = index + 1
		}
	}
	return flush(len(value))
}

func writeResponsesString(writer io.Writer, value string) error {
	for len(value) > 0 {
		written, err := io.WriteString(writer, value)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}

func writeResponsesBytes(writer io.Writer, value []byte) error {
	for len(value) > 0 {
		written, err := writer.Write(value)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}
