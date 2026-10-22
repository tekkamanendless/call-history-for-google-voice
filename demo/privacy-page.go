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
			app.Text("Last updated: 2026-10-21"),
			app.P().Body(
				app.Text("This a truly local web app; it consists of static files."),
				app.Text(" "),
				app.Text("There is no server component."),
			),
			app.P().Body(
				app.Text("No information gathered here will be sent anywhere."),
			),
			app.P().Body(
				app.Text("It only makes requests on your behalf to various Google APIs."),
			),
			app.H3().Text("1. Information We Do Not Collect"),
			app.Text("We respect your privacy. We do not collect, store, or process any personal information, usage data, or device identifiers from visitors or users of this site."),
			app.H3().Text("2. Information We Do Not Share"),
			app.Text("Because we do not collect any personal data, we do not sell, trade, or share any information with third parties."),
			app.H3().Text("3. Cookies and Tracking"),
			app.Text("We do not use cookies, analytics tools, or tracking technologies."),
			app.H3().Text("4. Changes to This Policy"),
			app.Text("We may update this privacy policy from time to time. Any changes will be posted on this page with an updated revision date."),
			app.H3().Text("5. Contact Us"),
			app.Text("If you have any questions about this privacy policy, you can contact us at: https://github.com/tekkamanendless"),
		)
}
