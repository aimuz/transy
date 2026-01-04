package clipboard

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func GetText(app *application.App) (string, error) {
	return getClipboardContent(app)
}
