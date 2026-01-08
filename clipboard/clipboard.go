package clipboard

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func GetText(app *application.App) (string, error) {
	if app == nil {
		return "", errors.New("app is nil")
	}
	text, ok := app.Clipboard.Text()
	if !ok {
		return "", errors.New("failed to get clipboard content")
	}
	return text, nil
}
