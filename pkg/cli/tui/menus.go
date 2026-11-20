package tui

// FooterMenu is an inline footer menu (never huh overlay).
type FooterMenu int

const (
	MenuNone FooterMenu = iota
	MenuActions
	MenuRoles
	MenuView
	MenuModels
	MenuChats
)

const (
	ActionNewChat = iota
	ActionModels
	ActionChats
	ActionTeams
	ActionReload
	ActionInit
	ActionExit
)

var actionLabels = []string{
	"Start New Chat",
	"Models",
	"Switch / List Chats",
	"Show Teams",
	"Reload Roles",
	"Init Workspace",
	"Exit",
}

// MenuTrigger reports whether a compose draft may open a footer menu.
func MenuTrigger(draft string, key rune) FooterMenu {
	if draft != "" {
		return MenuNone
	}
	switch key {
	case '/':
		return MenuActions
	case '*':
		return MenuRoles
	case '=':
		return MenuView
	default:
		return MenuNone
	}
}

// MentionInsert formats catalog display name with trailing space.
func MentionInsert(displayName string) string {
	return "*" + displayName + " "
}

// MentionFallback converts a bare key after closing the picker into normal text.
func MentionFallback(key rune) string {
	return "*" + string(key)
}
