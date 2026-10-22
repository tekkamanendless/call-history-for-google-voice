package demo

import (
	"log/slog"

	"github.com/go-app-blazar/blazar/blazar"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

type IndexPage struct {
	app.Compo
}

func (c *IndexPage) OnMount(ctx app.Context) {
	slog.InfoContext(ctx.Context, "IndexPage: OnMount")
}

func (c *IndexPage) OnNav(ctx app.Context) {
	slog.InfoContext(ctx.Context, "IndexPage: OnNav")
}

func (c *IndexPage) Render() app.UI {
	return blazar.Page().
		Body(
			app.P().Text("This tool uses the Google Admin API to show a graphical view of your organization's Google Voice calls."),
			blazar.Button().
				To("/calls").
				Label("Get Started"),
			app.Br(),
			app.A().
				Href("/terms").
				Text("Terms of Service"),
			app.Br(),
			app.A().
				Href("/privacy").
				Text("Privacy Policy"),
		)
}
