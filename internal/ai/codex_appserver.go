package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// codexFallbackEfforts se ofrecen si no se puede leer el catálogo de Codex.
var codexFallbackEfforts = []string{"low", "medium", "high"}

// codexCatalogTimeout limita la consulta a `codex app-server`; variable para los tests.
var codexCatalogTimeout = 5 * time.Second

// codexModelList es la parte que usamos de la respuesta de `model/list`.
type codexModelList struct {
	Data []struct {
		ID                        string `json:"id"`
		DisplayName               string `json:"displayName"`
		Hidden                    bool   `json:"hidden"`
		IsDefault                 bool   `json:"isDefault"`
		DefaultReasoningEffort    string `json:"defaultReasoningEffort"`
		SupportedReasoningEfforts []struct {
			ReasoningEffort string `json:"reasoningEffort"`
		} `json:"supportedReasoningEfforts"`
	} `json:"data"`
}

// codexConfig es la parte que usamos de la respuesta de `config/read`.
type codexConfig struct {
	Config struct {
		Model  string `json:"model"`
		Effort string `json:"model_reasoning_effort"`
	} `json:"config"`
}

// Catalog consulta a `codex app-server` los modelos disponibles y la configuración
// efectiva (config.toml, perfiles). Si falla, devuelve solo esfuerzos genéricos y Codex
// usará lo suyo.
func (c Codex) Catalog(ctx context.Context) Catalog {
	cat := Catalog{DefaultEfforts: codexFallbackEfforts}
	res, err := codexAppServer(ctx, c.Binary(),
		rpcCall{Method: "model/list", Params: struct{}{}},
		rpcCall{Method: "config/read", Params: struct{}{}})
	if err != nil {
		return cat
	}

	var list codexModelList
	if json.Unmarshal(res[0], &list) == nil {
		for _, m := range list.Data {
			if m.Hidden || m.ID == "" {
				continue
			}
			model := Model{ID: m.ID, Name: m.DisplayName, DefaultEffort: m.DefaultReasoningEffort}
			if model.Name == "" {
				model.Name = m.ID
			}
			for _, e := range m.SupportedReasoningEfforts {
				model.Efforts = append(model.Efforts, e.ReasoningEffort)
			}
			cat.Models = append(cat.Models, model)
			if m.IsDefault {
				cat.defaultID = m.ID
				cat.DefaultModel = Setting{Value: model.Name, Source: "predeterminado de Codex"}
			}
		}
	}

	var cfg codexConfig
	if json.Unmarshal(res[1], &cfg) == nil {
		if cfg.Config.Model != "" {
			cat.defaultID = cfg.Config.Model
			cat.DefaultModel = Setting{Value: cfg.Config.Model, Source: "configurado en Codex"}
		}
		if cfg.Config.Effort != "" {
			cat.Effort = Setting{Value: cfg.Config.Effort, Source: "configurado en Codex"}
		}
	}

	for _, m := range cat.Models {
		if m.ID == cat.defaultID {
			cat.DefaultModel.Value = m.Name
			if len(m.Efforts) > 0 {
				cat.DefaultEfforts = m.Efforts
			}
		}
	}
	return cat
}

type rpcCall struct {
	Method string
	Params any
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// codexAppServer abre una sesión JSON-RPC (una línea por mensaje) con `codex app-server`,
// hace calls y devuelve sus resultados en el mismo orden.
func codexAppServer(ctx context.Context, bin string, calls ...rpcCall) ([]json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, codexCatalogTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "app-server")
	// `codex` es un script de Node que lanza el binario real: se mata todo el grupo.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("codex app-server: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex app-server: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		cancel()
		_ = cmd.Wait() // termina por la cancelación; el error no aporta
	}()

	enc := json.NewEncoder(stdin)
	id := func(n int) *int { return &n }
	msgs := []rpcMessage{
		{JSONRPC: "2.0", ID: id(0), Method: "initialize", Params: map[string]any{
			"clientInfo": map[string]string{"name": "meet2do", "version": "1"}}},
		{JSONRPC: "2.0", Method: "initialized"},
	}
	for i, c := range calls {
		msgs = append(msgs, rpcMessage{JSONRPC: "2.0", ID: id(i + 1), Method: c.Method, Params: c.Params})
	}
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return nil, fmt.Errorf("codex app-server: %w", err)
		}
	}

	results := make([]json.RawMessage, len(calls))
	pending := len(calls)
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	for pending > 0 && sc.Scan() {
		var m rpcMessage
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.ID == nil || *m.ID < 1 || *m.ID > len(calls) {
			continue
		}
		if m.Error != nil {
			return nil, fmt.Errorf("codex app-server: %s: %s", calls[*m.ID-1].Method, m.Error.Message)
		}
		if results[*m.ID-1] == nil {
			pending--
		}
		results[*m.ID-1] = m.Result
	}
	if pending > 0 {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("codex app-server: %w", err)
		}
		return nil, errors.New("codex app-server: terminó sin responder")
	}
	return results, nil
}
