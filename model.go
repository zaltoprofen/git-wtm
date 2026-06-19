package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type uiMode int

const (
	modeList uiMode = iota
	modeCreate
	modeConfirmDelete
)

type loadStateMsg struct {
	state         appState
	status        string
	preferredPath string
	err           error
}

type actionErrorMsg struct {
	err error
}

type baseRefOptionsMsg struct {
	refs          []string
	localBranches []string
	err           error
}

type createMode int

const (
	createModeNewBranch createMode = iota
	createModeExistingLocalBranch
)

type createForm struct {
	inputs           []textinput.Model
	focus            int
	allBaseRefs      []string
	localBranches    map[string]struct{}
	filteredBaseRefs []string
	suggestionIndex  int
}

type exitAction int

const (
	exitActionNone exitAction = iota
	exitActionOpenShell
)

type model struct {
	state            appState
	cursor           int
	mode             uiMode
	create           createForm
	exitAction       exitAction
	exitPath         string
	pendingDelete    worktreeEntry
	hasPendingDelete bool
	status           string
	statusIsError    bool
	busy             bool
	width            int
	height           int
}

func newModel() model {
	branchInput := textinput.New()
	branchInput.Placeholder = "feature/new-worktree"
	branchInput.CharLimit = 255
	branchInput.Width = 40

	baseRefInput := textinput.New()
	baseRefInput.Placeholder = "main"
	baseRefInput.CharLimit = 255
	baseRefInput.Width = 40

	return model{
		create: createForm{inputs: []textinput.Model{branchInput, baseRefInput}},
		busy:   true,
	}
}

func (m model) Init() tea.Cmd {
	return refreshCmd("", "")
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case loadStateMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}

		m.state = msg.state
		m.applySelection(msg.preferredPath)
		if msg.status != "" {
			m.setStatus(msg.status, false)
		} else if m.status == "" {
			m.setStatus("Ready", false)
		}
		return m, nil
	case actionErrorMsg:
		m.busy = false
		m.setStatus(msg.err.Error(), true)
		return m, nil
	case baseRefOptionsMsg:
		if msg.err != nil {
			m.create.allBaseRefs = nil
			m.create.localBranches = nil
			m.create.filteredBaseRefs = nil
			m.create.suggestionIndex = 0
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}

		m.create.allBaseRefs = msg.refs
		m.create.localBranches = make(map[string]struct{}, len(msg.localBranches))
		for _, branch := range msg.localBranches {
			m.create.localBranches[branch] = struct{}{}
		}
		m.refreshBaseRefSuggestions()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.mode != modeCreate {
				return m, tea.Quit
			}
		}
	}

	if m.busy {
		return m, nil
	}

	switch m.mode {
	case modeCreate:
		return m.updateCreate(msg)
	case modeConfirmDelete:
		return m.updateConfirmDelete(msg)
	default:
		return m.updateList(msg)
	}
}

func (m model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.state.Worktrees)-1 {
			m.cursor++
		}
	case "enter", "o":
		entry, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.exitAction = exitActionOpenShell
		m.exitPath = entry.Path
		return m, tea.Quit
	case "c":
		m.mode = modeCreate
		m.resetCreateForm()
		m.setStatus("Enter a branch name for the new worktree", false)
		return m, tea.Batch(textinput.Blink, loadBaseRefOptionsCmd())
	case "d":
		entry, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		if entry.IsMain {
			m.setStatus("Primary worktree cannot be removed", true)
			return m, nil
		}
		if entry.IsCurrent {
			m.setStatus("Current worktree cannot be removed", true)
			return m, nil
		}
		m.mode = modeConfirmDelete
		m.pendingDelete = entry
		m.hasPendingDelete = true
		return m, nil
	case "r":
		m.busy = true
		m.setStatus("Refreshing worktrees...", false)
		return m, refreshCmd("", m.selectedPath())
	}

	return m, nil
}

func (m model) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, 2)
	keyMsg, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch keyMsg.String() {
		case "esc":
			m.mode = modeList
			m.blurCreateForm()
			m.setStatus("Create canceled", false)
			return m, nil
		case "tab":
			if m.create.focus == 1 && m.currentCreateMode() == createModeNewBranch && m.hasBaseRefSuggestions() {
				m.acceptSelectedBaseRefSuggestion()
				return m, nil
			}
			m.moveCreateFocus(keyMsg.String())
			return m, nil
		case "shift+tab":
			m.moveCreateFocus(keyMsg.String())
			return m, nil
		case "up", "down":
			if m.create.focus == 1 && m.currentCreateMode() == createModeNewBranch && m.hasBaseRefSuggestions() {
				m.moveBaseRefSuggestion(keyMsg.String())
				return m, nil
			}
			m.moveCreateFocus(keyMsg.String())
			return m, nil
		case "enter":
			branchName := strings.TrimSpace(m.create.inputs[0].Value())
			if branchName == "" {
				m.setStatus("Branch name is required", true)
				return m, nil
			}
			baseRef := m.effectiveBaseRef()

			m.mode = modeList
			m.blurCreateForm()
			m.busy = true
			m.setStatus("Creating worktree...", false)
			return m, createWorktreeCmd(branchName, baseRef, m.state.Repo)
		}
	}

	for index := range m.create.inputs {
		m.create.inputs[index], _ = m.create.inputs[index].Update(msg)
	}
	m.refreshBaseRefSuggestions()
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

func (m model) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "esc", "n":
		m.mode = modeList
		m.hasPendingDelete = false
		m.setStatus("Delete canceled", false)
		return m, nil
	case "enter", "y":
		if !m.hasPendingDelete {
			m.mode = modeList
			return m, nil
		}
		entry := m.pendingDelete
		m.mode = modeList
		m.hasPendingDelete = false
		m.busy = true
		m.setStatus("Removing worktree...", false)
		return m, removeWorktreeCmd(entry)
	}

	return m, nil
}

func (m model) View() string {
	var builder strings.Builder
	builder.WriteString("git-wtm\n\n")

	if m.state.Repo.RepoName != "" {
		builder.WriteString(fmt.Sprintf("Repository: %s\n", m.state.Repo.RepoName))
		builder.WriteString(fmt.Sprintf("Primary worktree: %s\n", m.state.Repo.RepoDir))
		builder.WriteString(fmt.Sprintf("New worktrees default to: %s\n\n", m.state.Repo.WorktreesDir))
	}

	builder.WriteString("Keys: up/down or j/k move, enter/o open shell, c create, d delete, r refresh, q quit\n\n")
	builder.WriteString(fmt.Sprintf("Worktrees (%d)\n", len(m.state.Worktrees)))

	if len(m.state.Worktrees) == 0 {
		builder.WriteString("  No worktrees found\n")
	} else {
		for index, entry := range m.state.Worktrees {
			builder.WriteString(m.renderWorktree(index == m.cursor, entry))
			builder.WriteByte('\n')
		}
	}

	builder.WriteByte('\n')
	if m.mode == modeCreate {
		builder.WriteString("Create worktree\n")
		builder.WriteString(fmt.Sprintf("Branch name: %s\n", m.create.inputs[0].View()))
		builder.WriteString(fmt.Sprintf("Mode:        %s\n", m.currentCreateModeLabel()))
		builder.WriteString(fmt.Sprintf("Base ref:    %s\n", m.baseRefDisplayValue()))
		if m.create.focus == 1 && m.currentCreateMode() == createModeNewBranch {
			builder.WriteString(m.renderBaseRefSuggestions())
		}
		builder.WriteString(fmt.Sprintf("Path: %s\n", m.previewCreatePath()))
		builder.WriteString(fmt.Sprintf("Will run:    %s\n", m.createCommandPreview()))
		if m.currentCreateMode() == createModeExistingLocalBranch {
			builder.WriteString("Base ref is ignored because a local branch with this name already exists.\n")
		}
		builder.WriteString("Tab accepts a base ref suggestion. Shift+Tab switches fields. Enter creates the worktree. Esc cancels.\n\n")
	} else if m.mode == modeConfirmDelete && m.hasPendingDelete {
		builder.WriteString("Delete worktree\n")
		builder.WriteString(fmt.Sprintf("Target: %s (%s)\n", m.pendingDelete.Name, m.pendingDelete.Path))
		builder.WriteString("Press y or Enter to confirm. Press n or Esc to cancel.\n\n")
	}

	if m.busy {
		builder.WriteString("Status: running git command...\n")
	} else if m.status != "" {
		prefix := "Status"
		if m.statusIsError {
			prefix = "Error"
		}
		builder.WriteString(fmt.Sprintf("%s: %s\n", prefix, m.status))
	}

	return builder.String()
}

func (m *model) applySelection(preferredPath string) {
	if len(m.state.Worktrees) == 0 {
		m.cursor = 0
		return
	}

	if preferredPath != "" {
		cleanPreferred := filepath.Clean(preferredPath)
		for index, entry := range m.state.Worktrees {
			if entry.Path == cleanPreferred {
				m.cursor = index
				return
			}
		}
	}

	if m.cursor >= len(m.state.Worktrees) {
		m.cursor = len(m.state.Worktrees) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) selectedEntry() (worktreeEntry, bool) {
	if len(m.state.Worktrees) == 0 || m.cursor < 0 || m.cursor >= len(m.state.Worktrees) {
		return worktreeEntry{}, false
	}
	return m.state.Worktrees[m.cursor], true
}

func (m *model) selectedPath() string {
	entry, ok := m.selectedEntry()
	if !ok {
		return ""
	}
	return entry.Path
}

func (m *model) previewCreatePath() string {
	branchName := strings.TrimSpace(m.create.inputs[0].Value())
	if branchName == "" {
		branchName = "<branch>"
	}
	return filepath.Join(m.state.Repo.WorktreesDir, filepath.FromSlash(branchName))
}

func (m *model) renderWorktree(selected bool, entry worktreeEntry) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	flags := make([]string, 0, 3)
	if entry.IsCurrent {
		flags = append(flags, "current")
	}
	if entry.IsMain {
		flags = append(flags, "primary")
	}
	if entry.Detached {
		flags = append(flags, "detached")
	}

	details := entry.Path
	if len(flags) > 0 {
		details = fmt.Sprintf("%s [%s]", details, strings.Join(flags, ", "))
	}

	base := fmt.Sprintf("%s%-20s  %-24s  ", prefix, trimRunes(entry.Name, 20), trimRunes(entry.Branch, 24))
	available := m.width - runeLen(base)
	if available < 24 {
		available = 24
	}

	return base + truncateMiddle(details, available)
}

func (m *model) setStatus(message string, isError bool) {
	m.status = message
	m.statusIsError = isError
}

func refreshCmd(status string, preferredPath string) tea.Cmd {
	return func() tea.Msg {
		state, err := loadState()
		return loadStateMsg{state: state, status: status, preferredPath: preferredPath, err: err}
	}
}

func loadBaseRefOptionsCmd() tea.Cmd {
	return func() tea.Msg {
		refs, err := listBaseRefs()
		if err != nil {
			return baseRefOptionsMsg{err: err}
		}
		localBranches, err := listLocalBranches()
		return baseRefOptionsMsg{refs: refs, localBranches: localBranches, err: err}
	}
}

func createWorktreeCmd(branchName string, baseRef string, repo repoInfo) tea.Cmd {
	return func() tea.Msg {
		path, err := createWorktree(branchName, baseRef, repo)
		if err != nil {
			return actionErrorMsg{err: err}
		}

		state, loadErr := loadState()
		return loadStateMsg{
			state:         state,
			status:        fmt.Sprintf("Created worktree: %s", path),
			preferredPath: path,
			err:           loadErr,
		}
	}
}

func removeWorktreeCmd(entry worktreeEntry) tea.Cmd {
	return func() tea.Msg {
		if err := removeWorktree(entry); err != nil {
			return actionErrorMsg{err: err}
		}

		state, loadErr := loadState()
		return loadStateMsg{
			state:         state,
			status:        fmt.Sprintf("Removed worktree: %s", entry.Path),
			preferredPath: "",
			err:           loadErr,
		}
	}
}

func truncateMiddle(value string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 3 {
		return string(runes[:width])
	}

	head := (width - 1) / 2
	tail := width - 1 - head
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

func trimRunes(value string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return string(runes[:1])
	}

	return string(runes[:width-1]) + "…"
}

func runeLen(value string) int {
	return len([]rune(value))
}

func (m *model) resetCreateForm() {
	for index := range m.create.inputs {
		m.create.inputs[index].SetValue("")
		m.create.inputs[index].Blur()
	}
	m.create.focus = 0
	m.create.localBranches = nil
	m.create.filteredBaseRefs = nil
	m.create.suggestionIndex = 0
	m.create.inputs[0].Focus()
}

func (m *model) blurCreateForm() {
	for index := range m.create.inputs {
		m.create.inputs[index].Blur()
	}
}

func (m *model) moveCreateFocus(direction string) {
	lastIndex := len(m.create.inputs) - 1
	if lastIndex < 0 {
		return
	}

	switch direction {
	case "shift+tab", "up":
		if m.create.focus == 0 {
			m.create.focus = lastIndex
		} else {
			m.create.focus--
		}
	default:
		if m.create.focus == lastIndex {
			m.create.focus = 0
		} else {
			m.create.focus++
		}
	}

	for index := range m.create.inputs {
		if index == m.create.focus {
			m.create.inputs[index].Focus()
		} else {
			m.create.inputs[index].Blur()
		}
	}
	m.refreshBaseRefSuggestions()
}

func (m *model) refreshBaseRefSuggestions() {
	if m.currentCreateMode() != createModeNewBranch {
		m.create.filteredBaseRefs = nil
		m.create.suggestionIndex = 0
		return
	}

	query := strings.TrimSpace(m.create.inputs[1].Value())
	m.create.filteredBaseRefs = filterBaseRefSuggestions(m.create.allBaseRefs, query)
	if len(m.create.filteredBaseRefs) == 0 {
		m.create.suggestionIndex = 0
		return
	}
	if m.create.suggestionIndex >= len(m.create.filteredBaseRefs) {
		m.create.suggestionIndex = 0
	}
	if m.create.suggestionIndex < 0 {
		m.create.suggestionIndex = 0
	}
}

func filterBaseRefSuggestions(refs []string, query string) []string {
	if len(refs) == 0 {
		return nil
	}

	if query == "" {
		return append([]string(nil), refs...)
	}

	query = strings.ToLower(query)
	filtered := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.Contains(strings.ToLower(ref), query) {
			filtered = append(filtered, ref)
		}
	}

	return filtered
}

func (m *model) hasBaseRefSuggestions() bool {
	return len(m.create.filteredBaseRefs) > 0
}

func (m *model) acceptSelectedBaseRefSuggestion() {
	if !m.hasBaseRefSuggestions() {
		return
	}

	selected := m.create.filteredBaseRefs[m.create.suggestionIndex]
	m.create.inputs[1].SetValue(selected)
	m.create.suggestionIndex = 0
	m.refreshBaseRefSuggestions()
}

func (m *model) moveBaseRefSuggestion(direction string) {
	if !m.hasBaseRefSuggestions() {
		return
	}

	lastIndex := len(m.create.filteredBaseRefs) - 1
	switch direction {
	case "up":
		if m.create.suggestionIndex == 0 {
			m.create.suggestionIndex = lastIndex
		} else {
			m.create.suggestionIndex--
		}
	default:
		if m.create.suggestionIndex == lastIndex {
			m.create.suggestionIndex = 0
		} else {
			m.create.suggestionIndex++
		}
	}
}

func (m *model) renderBaseRefSuggestions() string {
	if !m.hasBaseRefSuggestions() {
		return "Base ref suggestions: none\n"
	}

	var builder strings.Builder
	builder.WriteString("Base ref suggestions:\n")
	limit := len(m.create.filteredBaseRefs)
	if limit > 5 {
		limit = 5
	}
	for index := 0; index < limit; index++ {
		prefix := "  "
		if index == m.create.suggestionIndex {
			prefix = "> "
		}
		builder.WriteString(prefix)
		builder.WriteString(m.create.filteredBaseRefs[index])
		builder.WriteByte('\n')
	}
	if len(m.create.filteredBaseRefs) > limit {
		builder.WriteString(fmt.Sprintf("  ... and %d more\n", len(m.create.filteredBaseRefs)-limit))
	}

	return builder.String()
}

func (m *model) currentCreateMode() createMode {
	branchName := strings.TrimSpace(m.create.inputs[0].Value())
	if branchName == "" {
		return createModeNewBranch
	}
	if m.create.localBranches == nil {
		return createModeNewBranch
	}
	if _, ok := m.create.localBranches[branchName]; ok {
		return createModeExistingLocalBranch
	}
	return createModeNewBranch
}

func (m *model) currentCreateModeLabel() string {
	switch m.currentCreateMode() {
	case createModeExistingLocalBranch:
		return "Use existing local branch"
	default:
		return "Create new branch"
	}
}

func (m *model) effectiveBaseRef() string {
	if m.currentCreateMode() == createModeExistingLocalBranch {
		return ""
	}
	return strings.TrimSpace(m.create.inputs[1].Value())
}

func (m *model) baseRefDisplayValue() string {
	value := strings.TrimSpace(m.create.inputs[1].View())
	if value == "" {
		value = m.create.inputs[1].View()
	}
	if m.currentCreateMode() == createModeExistingLocalBranch {
		inputValue := strings.TrimSpace(m.create.inputs[1].Value())
		if inputValue == "" {
			return "(ignored)"
		}
		return inputValue + " (ignored)"
	}
	return m.create.inputs[1].View()
}

func (m *model) createCommandPreview() string {
	branchName := strings.TrimSpace(m.create.inputs[0].Value())
	path := m.previewCreatePath()
	if branchName == "" {
		branchName = "<branch>"
	}

	if m.currentCreateMode() == createModeExistingLocalBranch {
		return fmt.Sprintf("git worktree add %s %s", path, branchName)
	}

	baseRef := m.effectiveBaseRef()
	if baseRef == "" {
		return fmt.Sprintf("git worktree add -b %s %s", branchName, path)
	}

	return fmt.Sprintf("git worktree add -b %s %s %s", branchName, path, baseRef)
}
