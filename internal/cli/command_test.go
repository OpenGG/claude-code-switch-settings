package cli

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs"
)

type stubPrompter struct {
	selects  []selectResponse
	prompts  []promptResponse
	confirms []confirmResponse

	selectCalls  int
	promptCalls  int
	confirmCalls int
}

type selectResponse struct {
	index int
	value string
	err   error
}

type promptResponse struct {
	value string
	err   error
}

type confirmResponse struct {
	value bool
	err   error
}

var errStubNoMore = errors.New("stub prompter: no more responses")

func (s *stubPrompter) Select(label string, items []string, defaultValue string) (int, string, error) {
	if s.selectCalls >= len(s.selects) {
		return 0, "", errStubNoMore
	}
	resp := s.selects[s.selectCalls]
	s.selectCalls++
	return resp.index, resp.value, resp.err
}

func (s *stubPrompter) Prompt(label string) (string, error) {
	if s.promptCalls >= len(s.prompts) {
		return "", errStubNoMore
	}
	resp := s.prompts[s.promptCalls]
	s.promptCalls++
	return resp.value, resp.err
}

func (s *stubPrompter) Confirm(label string, defaultYes bool) (bool, error) {
	if s.confirmCalls >= len(s.confirms) {
		return false, errStubNoMore
	}
	resp := s.confirms[s.confirmCalls]
	s.confirmCalls++
	return resp.value, resp.err
}

func newTestCommandManager(t *testing.T) *ccs.Manager {
	t.Helper()
	fs := afero.NewMemMapFs()
	mgr := ccs.NewManager(fs, "/home/test", nil) // nil logger = discard logger for tests
	if err := mgr.InitInfra(); err != nil {
		t.Fatalf("InitInfra: %v", err)
	}
	return mgr
}

func TestListCommandOutput(t *testing.T) {
	mgr := newTestCommandManager(t)
	if err := afero.WriteFile(mgr.FileSystem(), mgr.ActiveSettingsPath(), []byte("A"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := mgr.SetActiveSettings("work"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	path, err := mgr.StoredSettingsPath("work")
	if err != nil {
		t.Fatalf("stored path: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), path, []byte("A"), 0o644); err != nil {
		t.Fatalf("write store: %v", err)
	}

	buf := &bytes.Buffer{}
	cmd := newListCommand(mgr, buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE list: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "* [work] (active)") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestListCommandUnsavedOutput(t *testing.T) {
	mgr := newTestCommandManager(t)
	if err := afero.WriteFile(mgr.FileSystem(), mgr.ActiveSettingsPath(), []byte("pending"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := mgr.SetActiveSettings(""); err != nil {
		t.Fatalf("clear active: %v", err)
	}
	buf := &bytes.Buffer{}
	cmd := newListCommand(mgr, buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE list: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "* (Current settings.json is unsaved)") {
		t.Fatalf("expected unsaved message, got %s", output)
	}
}

func TestUseCommandInteractive(t *testing.T) {
	mgr := newTestCommandManager(t)
	path, err := mgr.StoredSettingsPath("work")
	if err != nil {
		t.Fatalf("stored path: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), path, []byte("stored"), 0o644); err != nil {
		t.Fatalf("write store: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), mgr.ActiveSettingsPath(), []byte("old"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}

	prompter := &stubPrompter{selects: []selectResponse{{value: "work"}}}
	buf := &bytes.Buffer{}
	cmd := newUseCommand(mgr, prompter, buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE use: %v", err)
	}
	content, err := afero.ReadFile(mgr.FileSystem(), mgr.ActiveSettingsPath())
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if string(content) != "stored" {
		t.Fatalf("expected stored content, got %s", content)
	}
}

func TestUseCommandArgument(t *testing.T) {
	mgr := newTestCommandManager(t)
	path, err := mgr.StoredSettingsPath("work")
	if err != nil {
		t.Fatalf("stored path: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), path, []byte("stored"), 0o644); err != nil {
		t.Fatalf("write store: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), mgr.ActiveSettingsPath(), []byte("old"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	mgr.SetNow(func() time.Time { return time.Unix(0, 0) })

	buf := &bytes.Buffer{}
	cmd := newUseCommand(mgr, &stubPrompter{}, buf)
	if err := cmd.RunE(cmd, []string{"work"}); err != nil {
		t.Fatalf("RunE use arg: %v", err)
	}
	info, err := mgr.FileSystem().Stat(mgr.ActiveSettingsPath())
	if err != nil {
		t.Fatalf("stat active: %v", err)
	}
	if info.ModTime().IsZero() {
		t.Fatalf("expected mod time update")
	}
}

func TestSaveCommandOverwriteFlow(t *testing.T) {
	mgr := newTestCommandManager(t)
	path, err := mgr.StoredSettingsPath("personal")
	if err != nil {
		t.Fatalf("stored path: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write store: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), mgr.ActiveSettingsPath(), []byte("Mod"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}

	prompter := &stubPrompter{
		selects:  []selectResponse{{value: "personal"}},
		confirms: []confirmResponse{{value: true}},
	}
	buf := &bytes.Buffer{}
	cmd := newSaveCommand(mgr, prompter)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE save: %v", err)
	}
	content, err := afero.ReadFile(mgr.FileSystem(), path)
	if err != nil {
		t.Fatalf("read personal: %v", err)
	}
	if string(content) != "Mod" {
		t.Fatalf("expected updated content, got %s", content)
	}
}

func TestSaveCommandNewValidation(t *testing.T) {
	mgr := newTestCommandManager(t)
	if err := afero.WriteFile(mgr.FileSystem(), mgr.ActiveSettingsPath(), []byte("data"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}

	prompter := &stubPrompter{
		selects: []selectResponse{{value: newSettingsLabel}},
		prompts: []promptResponse{{value: "my/settings"}, {value: "dev"}},
	}
	buf := &bytes.Buffer{}
	cmd := newSaveCommand(mgr, prompter)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE save new: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "invalid characters") {
		t.Fatalf("expected validation error output, got %s", output)
	}
	devPath, err := mgr.StoredSettingsPath("dev")
	if err != nil {
		t.Fatalf("stored path: %v", err)
	}
	exists, err := afero.Exists(mgr.FileSystem(), devPath)
	if err != nil {
		t.Fatalf("exists dev: %v", err)
	}
	if !exists {
		t.Fatalf("expected dev settings to be created")
	}
}

func TestPruneCommandInteractiveCancel(t *testing.T) {
	mgr := newTestCommandManager(t)
	prompter := &stubPrompter{selects: []selectResponse{{value: "Cancel"}}}
	buf := &bytes.Buffer{}
	cmd := newPruneCommand(mgr, prompter, buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE prune: %v", err)
	}
	if !strings.Contains(buf.String(), "Prune cancelled.") {
		t.Fatalf("expected cancel message")
	}
}

func TestParseHumanDuration(t *testing.T) {
	dur, err := parseHumanDuration("30d")
	if err != nil {
		t.Fatalf("parse 30d: %v", err)
	}
	if dur != 30*24*time.Hour {
		t.Fatalf("unexpected duration: %v", dur)
	}
}

func TestPruneCommandNonInteractive(t *testing.T) {
	mgr := newTestCommandManager(t)
	path := mgr.BackupDir()
	if err := afero.WriteFile(mgr.FileSystem(), filepath.Join(path, "keep.json"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write keep: %v", err)
	}
	cmd := newPruneCommand(mgr, &stubPrompter{}, bytes.NewBuffer(nil))
	cmd.Flags().Set("older-than", "1h")
	cmd.Flags().Set("force", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("prune non-interactive: %v", err)
	}
}

func TestParseHumanDurationInvalid(t *testing.T) {
	if _, err := parseHumanDuration(""); err == nil {
		t.Fatalf("expected error for empty value")
	}
	if _, err := parseHumanDuration("5x"); err == nil {
		t.Fatalf("expected error for unsupported suffix")
	}
	if _, err := parseHumanDuration("-1h"); err == nil {
		t.Fatalf("expected error for negative duration")
	}
}

func TestParseDaysInvalid(t *testing.T) {
	if _, err := parseDays("abc"); err == nil {
		t.Fatalf("expected parse error")
	}
	if _, err := parseDays("-1"); err == nil {
		t.Fatalf("expected negative error")
	}
}

func TestReorderWithDefault(t *testing.T) {
	items := []string{"a", "b", "c"}
	reordered := reorderWithDefault(items, "b")
	if reordered[0] != "b" {
		t.Fatalf("expected b first, got %v", reordered)
	}
	if reorderWithDefault(items, "")[0] != "a" {
		t.Fatalf("expected original order when default empty")
	}
	if reorderWithDefault(items, "a")[0] != "a" {
		t.Fatalf("expected original order when already first")
	}
}

func TestNewRootCommand(t *testing.T) {
	mgr := newTestCommandManager(t)
	root := NewRootCommand(mgr, &stubPrompter{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if root == nil {
		t.Fatalf("expected root command")
	}
	if len(root.Commands()) != 4 {
		t.Fatalf("expected 4 subcommands, got %d", len(root.Commands()))
	}
}

func TestPromptUISelect(t *testing.T) {
	stdin := bytes.NewBufferString("")
	pu := NewPromptUIWithIO(stdin, &nopWriteCloser{Writer: bytes.NewBuffer(nil)})
	_, _, err := pu.Select("choose", []string{"first", "second"}, "")
	if err == nil || !errors.Is(err, ErrPromptCancelled) {
		t.Fatalf("expected selection cancellation error")
	}
}

func TestPromptUISelectWithDefault(t *testing.T) {
	stdin := bytes.NewBufferString("")
	pu := NewPromptUIWithIO(stdin, &nopWriteCloser{Writer: bytes.NewBuffer(nil)})
	if _, _, err := pu.Select("choose", []string{"alpha", "beta"}, "beta"); err == nil || !errors.Is(err, ErrPromptCancelled) {
		t.Fatalf("expected selection cancellation error")
	}
}

func TestPromptUIPrompt(t *testing.T) {
	stdin := bytes.NewBufferString("")
	pu := NewPromptUIWithIO(stdin, &nopWriteCloser{Writer: bytes.NewBuffer(nil)})
	if _, err := pu.Prompt("enter"); err == nil || !errors.Is(err, ErrPromptCancelled) {
		t.Fatalf("expected prompt cancellation error")
	}
}

func TestPromptUIConfirm(t *testing.T) {
	stdin := bytes.NewBufferString("")
	pu := NewPromptUIWithIO(stdin, &nopWriteCloser{Writer: bytes.NewBuffer(nil)})
	if ok, err := pu.Confirm("confirm", false); err == nil || !errors.Is(err, ErrPromptCancelled) || ok {
		t.Fatalf("expected confirm cancellation")
	}
}

func TestPromptUIConfirmDefaultYes(t *testing.T) {
	stdin := bytes.NewBufferString("")
	pu := NewPromptUIWithIO(stdin, &nopWriteCloser{Writer: bytes.NewBuffer(nil)})
	if _, err := pu.Confirm("confirm", true); err == nil || !errors.Is(err, ErrPromptCancelled) {
		t.Fatalf("expected confirm cancellation with default")
	}
}

func TestToReadCloserPassthrough(t *testing.T) {
	reader := io.NopCloser(strings.NewReader("data"))
	if toReadCloser(reader) != reader {
		t.Fatalf("expected toReadCloser to return original read closer")
	}
	rc := toReadCloser(strings.NewReader("data"))
	if err := rc.Close(); err != nil {
		t.Fatalf("expected close to succeed: %v", err)
	}
}

func TestToWriteCloserPassthrough(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := nopWriteCloser{Writer: buf}
	if toWriteCloser(writer) != writer {
		t.Fatalf("expected toWriteCloser to return original write closer")
	}
	if _, err := toWriteCloser(buf).Write([]byte("hi")); err != nil {
		t.Fatalf("expected wrapped writer to accept data: %v", err)
	}
}

func TestNewPromptUIDefaults(t *testing.T) {
	pu := NewPromptUI()
	if pu == nil {
		t.Fatalf("expected prompt UI instance")
	}
	nop := nopWriteCloser{Writer: bytes.NewBuffer(nil)}
	if err := nop.Close(); err != nil {
		t.Fatalf("close should not error: %v", err)
	}
}

func TestSaveCommandOverwriteCancelled(t *testing.T) {
	mgr := newTestCommandManager(t)
	path, err := mgr.StoredSettingsPath("personal")
	if err != nil {
		t.Fatalf("stored path: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write store: %v", err)
	}
	if err := afero.WriteFile(mgr.FileSystem(), mgr.ActiveSettingsPath(), []byte("current"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	prompter := &stubPrompter{
		selects:  []selectResponse{{value: "personal"}},
		confirms: []confirmResponse{{value: false}},
	}
	buf := &bytes.Buffer{}
	cmd := newSaveCommand(mgr, prompter)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE save cancel: %v", err)
	}
	if !strings.Contains(buf.String(), "Aborted saving settings.") {
		t.Fatalf("expected cancel message")
	}
}

func TestUseCommandNoStoredSettings(t *testing.T) {
	mgr := newTestCommandManager(t)
	cmd := newUseCommand(mgr, &stubPrompter{}, bytes.NewBuffer(nil))
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatalf("expected error when no stored settings exist")
	}
}

func TestPruneCommandInteractiveConfirm(t *testing.T) {
	mgr := newTestCommandManager(t)
	backup := mgr.BackupDir()
	if err := afero.WriteFile(mgr.FileSystem(), filepath.Join(backup, "old.json"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	mgr.SetNow(func() time.Time { return time.Now().Add(48 * time.Hour) })
	prompter := &stubPrompter{
		selects:  []selectResponse{{value: "30d"}},
		confirms: []confirmResponse{{value: true}},
	}
	buf := &bytes.Buffer{}
	cmd := newPruneCommand(mgr, prompter, buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE prune confirm: %v", err)
	}
	if !strings.Contains(buf.String(), "Deleted") {
		t.Fatalf("expected deletion output")
	}
}
