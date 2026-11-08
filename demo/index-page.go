package demo

import (
	"log/slog"
	"strings"

	"github.com/go-app-blazar/blazar/blazar"
	"github.com/go-app-blazar/tabler-font"
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
			app.P().Body(
				blazar.Button().
					Style("font-size", "1.2em").
					Style("padding", "0.5em").
					To("/calls").
					Icon(tabler.IconFilledArrowBigRight).
					Label("Get Started"),
			),
			app.Br(),
			app.H2().Text("Google Workspace Setup"),
			app.Div().Body(
				app.P().Body(
					app.Text("This works with the enterprise version of Google Voice; it will not work with a personal Google Voice account. "),
					app.Text("This uses the Google Admin Reports API to query the list of call records and assembles them into a helpful format. "),
					app.Text("This is the same information that you can find under "),
					app.A().Href("https://admin.google.com/ac/sc/investigation?ref=reporting").Text("Reporting > Audit and Investigation"),
					app.Text(". "),
				),
				app.P().Body(
					app.Text("You'll need to use a Google Workspace account with the following permissions."),
				),
				app.Ol().Body(
					app.Li().Body(
						GoogleWorkspacePermission().
							Required(true).
							Parts("Security Center", "This user has full administrative rights for Security Center", "Audit and Investigation", "View", "Voice").
							Description("This provides access to the Google Voice call records."),
					),
					app.Li().Body(
						GoogleWorkspacePermission().
							Required(false).
							Parts("Directory Settings", "Read and update shared contacts from Domain Shared Contacts API").
							Description("This provides access to the organization's shared contacts.  If this permission is not granted, then no contact names will be shown."),
					),
				),
			),
			app.H2().Text("Additional Information"),
			app.Div().Body(
				app.P().Body(
					app.Text("For additional information, see below."),
				),
				app.Ul().Body(
					app.Li().Body(
						app.A().
							Href("/terms").
							Text("Terms of Service"),
					),
					app.Li().Body(
						app.A().
							Href("/privacy").
							Text("Privacy Policy"),
					),
				),
			),
		)
}

type googleWorkspacePermission struct {
	app.Compo

	IParts       []string
	IRequired    bool
	IDescription string
}

func GoogleWorkspacePermission() *googleWorkspacePermission {
	return &googleWorkspacePermission{}
}

func (p *googleWorkspacePermission) Parts(parts ...string) *googleWorkspacePermission {
	p.IParts = parts
	return p
}

func (p *googleWorkspacePermission) Required(required bool) *googleWorkspacePermission {
	p.IRequired = required
	return p
}

func (p *googleWorkspacePermission) Description(description string) *googleWorkspacePermission {
	p.IDescription = description
	return p
}

func (p *googleWorkspacePermission) Render() app.UI {
	var pathParts []string
	var name string
	if len(p.IParts) > 0 {
		pathParts = p.IParts[:len(p.IParts)-1]
		name = p.IParts[len(p.IParts)-1]
	}

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("gap", "0.2em").
		Style("margin-bottom", "0.5em").
		Body(
			app.Div().
				Style("display", "flex").
				Style("flex-direction", "column").
				Style("border", "1px solid #ccc").
				Style("padding", "0.5em").
				Style("border-radius", "0.5em").
				Body(
					app.Div().
						Class("google-workspace-permission__path").
						Style("font-size", "0.7em").
						Text(strings.Join(pathParts, " > ")),
					app.Div().
						Class("google-workspace-permission__name").
						Style("font-weight", "bold").
						Text(name),
				),
			app.Div().
				Class("google-workspace-permission__description").
				Style("font-style", "italic").
				Text(p.IDescription),
		)
}
