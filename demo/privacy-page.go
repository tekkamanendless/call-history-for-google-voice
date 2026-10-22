package demo

import (
	"log/slog"

	"github.com/go-app-blazar/blazar/blazar"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

type PrivacyPage struct {
	app.Compo
}

func (c *PrivacyPage) OnMount(ctx app.Context) {
	slog.InfoContext(ctx.Context, "PrivacyPage: OnMount")
}

func (c *PrivacyPage) OnNav(ctx app.Context) {
	slog.InfoContext(ctx.Context, "PrivacyPage: OnNav")
}

func (c *PrivacyPage) Render() app.UI {
	return blazar.Page().
		Body(
			app.H2().Text("Privacy Policy"),
			app.P().Body(
				app.Text("This a truly local web app; it consists of static files."),
				app.Text("There is no server component."),
			),
			app.P().Body(
				app.Text("No information gathered here will be sent anywhere."),
			),
			app.P().Body(
				app.Text("It only makes requests on your behalf to various Google APIs."),
			),
		)
}
