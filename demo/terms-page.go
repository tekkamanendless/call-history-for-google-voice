package demo

import (
	"log/slog"

	"github.com/go-app-blazar/blazar/blazar"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

type TermsPage struct {
	app.Compo
}

func (c *TermsPage) OnMount(ctx app.Context) {
	slog.InfoContext(ctx.Context, "TermsPage: OnMount")
}

func (c *TermsPage) OnNav(ctx app.Context) {
	slog.InfoContext(ctx.Context, "TermsPage: OnNav")
}

func (c *TermsPage) Render() app.UI {
	return blazar.Page().
		Body(
			app.H2().Text("Terms of Service"),
			app.P().Body(
				app.Text("This is provided as-is."),
				app.Text("It utilizes various Google APIs that may not work in the future; I'm not Google; I don't know."),
			),
			app.P().Body(
				app.Text("Do not use this for anything other than its intended purpose (visualizing Google Voice logs)."),
			),
		)
}
