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
			app.H3().Text("1. Information We Collect"),
			app.P().Body(
				app.Text("We respect your privacy."),
				app.Text(" "),
				app.Text("We do not collect, store, or process any personal information, usage data, or device identifiers from visitors or users of this site outside of your browser's local storage."),
				app.Text(" "),
				app.Text("When granted access to your Google account, the application will access the your profile information (including your name and e-mail address) as well as your organization's Google Voice call data (including all source and destination information, call times, etc.)."),
				app.Text(" "),
				app.Text("This data is used to display the calls in a graphical view."),
			),
			app.P().Body(
				app.Text("This data will only be stored in your browser's local storage as part of the HTML local storage API."),
			),
			app.H3().Text("2. Information We Share"),
			app.P().Body(
				app.Text("Because we do not collect any personal data, we do not sell, trade, or share any information with third parties."),
				app.Text(" "),
				app.Text("Any information gathered will only be stored in your browser's local storage."),
			),
			app.P().Body(
				app.Text("To delete your data, you can clear your browser's local storage for this domain."),
			),
			app.H3().Text("3. Cookies and Tracking"),
			app.P().Body(
				app.Text("We do not use cookies, analytics tools, or tracking technologies."),
				app.Text(" "),
				app.Text("This application is a static site hosted on GitHub Pages."),
			),
			app.H3().Text("4. Changes to This Policy"),
			app.P().Body(
				app.Text("We may update this privacy policy from time to time. Any changes will be posted on this page with an updated revision date."),
			),
			app.H3().Text("5. Contact Us"),
			app.P().Body(
				app.Text("If you have any questions about this privacy policy, you can contact us at: https://github.com/tekkamanendless"),
			),
		)
}
