package demo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/go-app-blazar/blazar/blazar"
	"github.com/go-app-blazar/blazar/htmlevent"
	"github.com/go-app-blazar/tabler-font"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"github.com/tekkamanendless/call-history-for-google-voice/googleoauthjs"
	"golang.org/x/oauth2"
	admin "google.golang.org/api/admin/reports/v1"
	"google.golang.org/api/googleapi"
	googleoauth2 "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

type CallsPage struct {
	app.Compo

	errorMessage string

	timezone    *time.Location
	authClient  *googleoauthjs.TokenClient
	userInfo    *googleoauth2.Userinfo
	userToken   *oauth2.Token
	adminClient *admin.Service

	configuration Configuration
	filter        Filter

	ready     bool
	loggingIn bool

	searching        bool
	searched         bool
	rawActivities    []*admin.Activity // Debug
	calls            []*Call           // Debug
	callRows         []CallRow
	filteredCallRows []CallRow
}

type Configuration struct {
	InternalVersion int    `json:"internalVersion"` // Used to track changes to the schema.
	Timeframe       string `json:"timeframe"`
	StartTimeString string `json:"startTimeString"`
	EndTimeString   string `json:"endTimeString"`
}

type Filter struct {
	InternalVersion     int    `json:"internalVersion"` // Used to track changes to the schema.
	Inbound             bool   `json:"inbound"`
	Outbound            bool   `json:"outbound"`
	ResultConnected     bool   `json:"resultConnected"`
	ResultIncomplete    bool   `json:"resultIncomplete"`
	ResultAutoAttendant bool   `json:"resultAutoAttendant"`
	ResultRingGroup     bool   `json:"resultRingGroup"`
	ResultPhone         bool   `json:"resultPhone"`
	ResultMessage       bool   `json:"resultMessage"`
	ResultVoicemail     bool   `json:"resultVoicemail"`
	PhoneNumber         string `json:"phoneNumber"`
}

func (c *CallsPage) OnMount(ctx app.Context) {
	slog.InfoContext(ctx.Context, "CallPage: OnMount")

	c.ready = false
	c.loggingIn = false

	c.configuration = Configuration{
		InternalVersion: 1,
		Timeframe:       "past-7-days",
		StartTimeString: time.Now().Add(-1 * time.Hour * 24).Format(time.DateOnly),
		EndTimeString:   "",
	}
	{
		var savedConfiguration Configuration
		ctx.GetState("configuration", &savedConfiguration)
		if savedConfiguration.InternalVersion != 0 {
			c.configuration = savedConfiguration
		}
	}

	c.filter = Filter{
		InternalVersion: 1,
	}
	{
		var savedFilter Filter
		ctx.GetState("filter", &savedFilter)
		if savedFilter.InternalVersion != 0 {
			c.filter = savedFilter
		}
	}

	c.timezone = time.Local

	{
		jsTimezone := app.Window().Get("Intl").Call("DateTimeFormat").Call("resolvedOptions").Get("timeZone").String()
		timezone, err := time.LoadLocation(jsTimezone)
		if err != nil {
			slog.ErrorContext(ctx.Context, "CallPage: OnMount: could not load timezone", "jsTimezone", jsTimezone, "err", err)
		} else {
			c.timezone = timezone
			slog.InfoContext(ctx.Context, "CallPage: OnMount: loaded timezone", "timezone", timezone)
		}
	}
	slog.InfoContext(ctx.Context, "CallPage: OnMount: timezone", "timezone", c.timezone)

	{
		loginCallback := func(token oauth2.Token) {
			slog.InfoContext(ctx.Context, "CallPage: OnMount: loginCallback", "token", token)
			c.userToken = &token
			ctx.SetState("user-token", *c.userToken).Persist()

			// NOTE: We're not going to do anything with the token; the rest of the login workflow is handled when needed.
		}

		errorCallback := func(err error) {
			slog.ErrorContext(ctx.Context, "CallPage: OnMount: errorCallback.", "err", err)
		}

		c.authClient = googleoauthjs.NewTokenClient(ctx.Context,
			googleoauthjs.WithClientID("531363734726-vvb6f23uehpuk2joljh46uj5igk745qv.apps.googleusercontent.com"),
			googleoauthjs.WithScopes([]string{
				"email",
				"profile",
				"https://www.googleapis.com/auth/admin.reports.audit.readonly",
			}),
			googleoauthjs.WithCallback(loginCallback),
			googleoauthjs.WithErrorCallback(errorCallback),
		)
	}

	ctx.Async(func() {
		ctx.GetState("user-token", &c.userToken)
		if c.userToken != nil && c.userToken.AccessToken != "" {
			slog.InfoContext(ctx.Context, "CallPage: OnMount: Saved token found.", "token", c.userToken)

			if c.tokenIsExpired(ctx) {
				slog.InfoContext(ctx.Context, "CallPage: OnMount: Token is expired, refreshing token.")

				err := c.refreshToken(ctx)
				if err != nil {
					slog.ErrorContext(ctx.Context, "CallPage: OnMount: Could not refresh token.", "err", err)
				} else {
					slog.InfoContext(ctx.Context, "CallPage: OnMount: Running initial search.")
					c.search(ctx)
				}
			} else {
				slog.InfoContext(ctx.Context, "CallPage: OnMount: Token is not expired, logging in with saved token.")

				err := c.setTokenAsPartOfLoginWorkflow(ctx, *c.userToken)
				if err != nil {
					slog.ErrorContext(ctx.Context, "CallPage: OnMount: Could not log in with saved token.", "err", err)
				} else {
					slog.InfoContext(ctx.Context, "CallPage: OnMount: Running initial search.")
					c.search(ctx)
				}
			}
		}

		c.ready = true
		slog.InfoContext(ctx.Context, "CallPage: OnMount: Ready.")
		ctx.Update()
	})
}

func (c *CallsPage) OnNav(ctx app.Context) {
	slog.InfoContext(ctx.Context, "CallPage: OnNav")
}

func (c *CallsPage) Render() app.UI {
	return blazar.Page().
		Body(
			blazar.Snackbar().
				Observe("snackbar-message"),
			app.IfSlice(!c.ready, func() []app.UI {
				return []app.UI{
					app.Div().Text("Loading..."),
				}
			}).ElseSlice(func() []app.UI {
				return []app.UI{
					app.If(c.ready && c.userInfo == nil, func() app.UI {
						return app.Div().
							Body(
								blazar.Button().
									Label("Sign in with Google").
									Icon(tabler.IconFilledBrandGoogle).
									Disabled(c.loggingIn).
									OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
										ctx.Async(func() {
											err := c.login(ctx)
											if err != nil {
												slog.ErrorContext(ctx.Context, "CallPage: Render: could not login.", "err", err)
												blazar.SendSnackbarMessage(ctx, "snackbar-message", blazar.SnackbarMessage().Closable(true).Bad().Text("Could not sign in with Google: "+err.Error()))
												return
											}

											ctx.Defer(func(ctx app.Context) {
												slog.InfoContext(ctx.Context, "CallPage: Running search after Google login.")
												c.search(ctx)
											})
										})
									}),
							)
					}),
					app.If(c.ready && c.userInfo != nil, func() app.UI {
						/*
							return app.Div().
								Body(
									app.Img().Src(c.userInfo.Picture).Width(32).Height(32),
									app.Span().Text(c.userInfo.Name),
								)
						*/
						return app.Div().
							Body(
								blazar.Button().
									Label("Log out as " + c.userInfo.Name).
									Icon(tabler.IconOutlineLogout).
									OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
										c.logout(ctx)
										ctx.Update()
									}),
							)
					}),
					app.IfSlice(c.ready && c.userInfo != nil, func() []app.UI {
						return []app.UI{
							blazar.Collapse().
								Label("Configuration").
								Open(true).
								Body(
									blazar.Form().
										Body(
											blazar.Select().
												Label("Timeframe").
												AllowedValue(
													blazar.SelectOption{
														Label: "Past 24 hours",
														Value: "past-24-hours",
													},
													blazar.SelectOption{
														Label: "Past 7 days",
														Value: "past-7-days",
													},
													blazar.SelectOption{
														Label: "Past 30 days",
														Value: "past-30-days",
													},
													blazar.SelectOption{
														Label: "Past 90 days",
														Value: "past-90-days",
													},
													blazar.SelectOption{
														Label: "Past 180 days",
														Value: "past-180-days",
													},
													blazar.SelectOption{
														Label: "Custom...",
														Value: "custom",
													},
												).
												Bind(&c.configuration.Timeframe).
												On("change", func(ctx app.Context, e app.Event) {
													ctx.SetState("configuration", c.configuration).Persist()
												}),
											app.IfSlice(c.configuration.Timeframe == "custom", func() []app.UI {
												return []app.UI{
													blazar.Input[string]().
														Label("Start Time").
														Clearable(true).
														Type("date").
														Bind(&c.configuration.StartTimeString).
														On("change", func(ctx app.Context, e app.Event) {
															ctx.SetState("configuration", c.configuration).Persist()
														}),
													blazar.Input[string]().
														Label("End Time").
														Clearable(true).
														Type("date").
														Bind(&c.configuration.EndTimeString).
														On("change", func(ctx app.Context, e app.Event) {
															ctx.SetState("configuration", c.configuration).Persist()
														}),
												}
											}),
										).
										Action(blazar.FormAction{
											Name:     "Search",
											Icon:     tabler.IconOutlineSearch,
											Function: c.search,
										}),
								),
							blazar.ProgressBar().
								Style("visibility", func() string {
									if c.searching {
										return "visible"
									}
									return "hidden"
								}()).
								Indeterminate(true),
							app.If(c.errorMessage != "", func() app.UI {
								return blazar.StatusBar().
									Bad().
									Text(c.errorMessage)
							}),
							app.If(c.searched, func() app.UI {
								return app.Div().
									Style("font-size", "0.8em").
									Style("gap", "1em").
									Style("display", "flex").
									Style("flex-direction", "column").
									Body(
										app.H2().Text("Call Table"),
										c.renderCallFilter(),
										c.renderCallTable(c.filteredCallRows),
									)
							}),
							app.If(c.searched, func() app.UI {
								return blazar.Collapse().
									Label("Debug").
									Open(false).
									Body(
										app.Div().
											Style("font-size", "0.8em").
											Body(
												app.H2().Text("Calls"),
												c.renderCalls(c.calls),
											),
										app.Div().
											Style("font-size", "0.8em").
											Body(
												app.H2().Text("Raw Activities"),
												app.Range(c.rawActivities).Slice(func(i int) app.UI {
													rawActivity := c.rawActivities[i]

													return app.Div().
														Body(
															app.Span().Text(rawActivity.Id.Time),
															app.Text(" "),
															app.Span().Text(rawActivity.Actor.Email),
															app.Text(" "),
															app.Span().Text(fmt.Sprintf("Events: (%d)", len(rawActivity.Events))),
															app.Range(rawActivity.Events).Slice(func(i int) app.UI {
																rawEvent := rawActivity.Events[i]
																parameters := map[string]*admin.ActivityEventsParameters{}
																for _, parameter := range rawEvent.Parameters {
																	parameters[parameter.Name] = parameter
																}

																if rawEvent.Type == "EVENT_TYPE_PBX_AUTO_ATTENDANT" && rawEvent.Name == "AUTO_ATTENDANT_PUBLISHED" {
																	return nil
																}

																return app.Div().
																	Style("padding-left", "1em").
																	Body(
																		app.Span().Text(parameters["PARAM_SOURCE"].Value+" -> "+parameters["PARAM_DESTINATION"].Value),
																		app.Br(),
																		app.Span().Text(rawEvent.Type),
																		app.Text(" | "),
																		app.Span().Text(rawEvent.Name),
																		app.Text(" ("+rawEvent.Status.EventStatus+")"),
																		app.Br(),
																		app.If(parameters["PARAM_RING_GROUP_NAME"] != nil, func() app.UI {
																			return app.Span().Text("Ring Group: " + parameters["PARAM_RING_GROUP_NAME"].Value)
																		}),
																		app.If(parameters["PARAM_RING_GROUP_NAME"] != nil, func() app.UI {
																			return app.Br()
																		}),
																		app.If(parameters["PARAM_DISTRIBUTION_ID"] != nil, func() app.UI {
																			return app.Span().Text("Distribution ID: " + parameters["PARAM_DISTRIBUTION_ID"].Value)
																		}),
																		app.If(parameters["PARAM_DISTRIBUTION_ID"] != nil, func() app.UI {
																			return app.Br()
																		}),
																		app.If(parameters["PARAM_AUTO_ATTENDANT_NAME"] != nil, func() app.UI {
																			return app.Span().Text("Auto Attendant: " + parameters["PARAM_AUTO_ATTENDANT_NAME"].Value)
																		}),
																		app.If(parameters["PARAM_AUTO_ATTENDANT_NAME"] != nil, func() app.UI {
																			return app.Br()
																		}),
																		app.If(parameters["PARAM_DURATION"] != nil && parameters["PARAM_DURATION"].IntValue > 0, func() app.UI {
																			return app.Span().Text("Duration: " + time.Duration(time.Duration(parameters["PARAM_DURATION"].IntValue)*time.Millisecond).String())
																		}),
																		app.If(parameters["PARAM_DURATION"] != nil && parameters["PARAM_DURATION"].IntValue > 0, func() app.UI {
																			return app.Span().Text(" ")
																		}),
																		app.If(parameters["PARAM_RING_DURATION"] != nil && parameters["PARAM_RING_DURATION"].IntValue > 0, func() app.UI {
																			return app.Span().Text("Ring Duration: " + time.Duration(time.Duration(parameters["PARAM_RING_DURATION"].IntValue)*time.Millisecond).String())
																		}),
																		app.If(parameters["PARAM_RING_DURATION"] != nil && parameters["PARAM_RING_DURATION"].IntValue > 0, func() app.UI {
																			return app.Span().Text(" ")
																		}),
																	)
															}),
														)
												}),
											),
									)
							}),
						}
					}),
				}
			}),
		)
}

const (
	DirectionIn  = "in"
	DirectionOut = "out"
)

const (
	SequenceAutoAttendant = "auto-attendant" // Initial/intermediate state.
	SequenceRingGroup     = "ring-group"     // Initial/intermediate state.
	SequencePhone         = "phone"          // Initial/intermediate state.
	SequenceMessage       = "message"        // Final state.
	SequenceVoicemail     = "voicemail"      // Final state.
	SequenceIncomplete    = "incomplete"     // Final state.
	SequenceConnected     = "connected"      // Final state.
)

func sequenceItemToIcon(item string) string {
	switch item {
	case SequenceAutoAttendant:
		return tabler.IconFilledSitemap
	case SequenceRingGroup:
		return tabler.IconOutlineUsers
	case SequencePhone:
		return tabler.IconFilledPhone
	case SequenceMessage:
		return tabler.IconOutlineMessage
	case SequenceVoicemail:
		return tabler.IconOutlineRecordMail
	case SequenceIncomplete:
		return tabler.IconOutlinePhoneOff
	case SequenceConnected:
		return tabler.IconFilledPhoneCall
	}
	return item
}

type CallRow struct {
	Time            time.Time     // The time of the call.
	Direction       string        // The direction of the call.
	Source          string        // The source phone number.
	SourceName      string        // The name of the source.
	Destination     string        // The destination phone number.
	DestinationName string        // The name of the destination.
	Duration        time.Duration // The duration of the call.
	ResultUser      string
	Sequence        []string // The sequence of states for the call.
	Calls           []*Call  // The calls that are part of the call.
}

func (c *CallsPage) renderCallFilter() app.UI {
	return blazar.Card().
		HeadlineText("Filter").
		Body(
			app.Div().
				Style("font-size", "0.9em").
				Style("display", "flex").
				Style("flex-direction", "row").
				Style("flex-wrap", "wrap").
				Style("gap", "1em").
				Body(
					app.Div().
						Body(
							blazar.Button().
								Icon(tabler.IconOutlineArrowLeft).
								Label("Inbound").
								Outline(!c.filter.Inbound).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.Inbound = !c.filter.Inbound
									c.filter.Outbound = false
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
							blazar.Button().
								Icon(tabler.IconOutlineArrowRight).
								Label("Outbound").
								Outline(!c.filter.Outbound).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.Inbound = false
									c.filter.Outbound = !c.filter.Outbound
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
						),
					app.Div().
						Body(
							blazar.Button().
								Icon(sequenceItemToIcon(SequenceAutoAttendant)).
								Label("Auto Attendant").
								Outline(!c.filter.ResultAutoAttendant).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.ResultAutoAttendant = !c.filter.ResultAutoAttendant
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
							blazar.Button().
								Icon(sequenceItemToIcon(SequenceRingGroup)).
								Label("Ring Group").
								Outline(!c.filter.ResultRingGroup).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.ResultRingGroup = !c.filter.ResultRingGroup
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
							blazar.Button().
								Icon(sequenceItemToIcon(SequencePhone)).
								Label("Phone").
								Outline(!c.filter.ResultPhone).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.ResultPhone = !c.filter.ResultPhone
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
							blazar.Button().
								Icon(sequenceItemToIcon(SequenceMessage)).
								Label("Message").
								Outline(!c.filter.ResultMessage).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.ResultMessage = !c.filter.ResultMessage
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
							blazar.Button().
								Icon(sequenceItemToIcon(SequenceVoicemail)).
								Label("Voicemail").
								Outline(!c.filter.ResultVoicemail).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.ResultVoicemail = !c.filter.ResultVoicemail
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
							blazar.Button().
								Icon(sequenceItemToIcon(SequenceIncomplete)).
								Label("Incomplete").
								Outline(!c.filter.ResultIncomplete).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.ResultIncomplete = !c.filter.ResultIncomplete
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
							blazar.Button().
								Icon(sequenceItemToIcon(SequenceConnected)).
								Label("Connected").
								Outline(!c.filter.ResultConnected).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									c.filter.ResultConnected = !c.filter.ResultConnected
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
						),
					app.If(c.filter.PhoneNumber != "", func() app.UI {
						return app.Div().
							Body(
								blazar.Button().
									Icon(sequenceItemToIcon(SequencePhone)).
									Label(formatPhoneNumber(c.filter.PhoneNumber)).
									Outline(false).
									OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
										c.filter.PhoneNumber = ""
										ctx.SetState("filter", c.filter).Persist()

										c.filteredCallRows = c.filterCallRows(c.callRows)
										ctx.Update()
									}),
							)
					}),
				),
		)
}

func (c *CallsPage) renderCallSide(side string, phoneNumber string, name string) app.UI {
	body := []app.UI{}
	if name != "" {
		body = append(body,
			app.Span().
				Class("name").
				Text(name),
		)
	}
	body = append(body,
		blazar.Copy().
			Style("text-wrap", "nowrap").
			Value(formatPhoneNumber(phoneNumber)).
			Text(formatPhoneNumber(phoneNumber)).
			OnCopy(func(ctx app.Context, value string) {
				blazar.SendSnackbarMessage(ctx, "snackbar-message", blazar.SnackbarMessage().Closable(true).Duration(3*time.Second).Text("Copied to clipboard: "+value))
			}),
	)
	return app.Div().
		Class("call-side " + side).
		Body(body...)
}

func (c *CallsPage) renderCallTable(callRows []CallRow) app.UI {
	return blazar.Table[CallRow]().
		Columns([]blazar.TableColumn[CallRow]{
			{
				Name: "Time",
				Value: func(row CallRow) any {
					return row.Time.In(c.timezone).Format("2006-01-02 3:04:05pm")
				},
			},
			{
				Name: "Direction",
				Value: func(row CallRow) any {
					icon := ""
					switch row.Direction {
					case DirectionIn:
						icon = tabler.IconOutlineArrowLeft
					case DirectionOut:
						icon = tabler.IconOutlineArrowRight
					}
					if icon == "" {
						return row.Direction
					}
					return blazar.Icon().
						Class("direction-" + row.Direction).
						Icon(icon)
				},
			},
			{
				Name: "Icons",
				Value: func(row CallRow) any {
					return app.Div().
						Style("text-wrap", "nowrap").
						Body(
							app.Range(row.Sequence).Slice(func(i int) app.UI {
								item := row.Sequence[i]
								icon := sequenceItemToIcon(item)
								return blazar.Icon().
									Style("width", "1em").
									Style("height", "1em").
									Style("margin-right", "0.5em").
									Class("result-" + item).
									Title(item).
									Icon(icon)
							}),
						)
				},
			},
			{
				Name: "Source",
				Value: func(row CallRow) any {
					if row.Source == "" {
						return ""
					}
					return app.Div().
						Class("call-side-container").
						Body(
							c.renderCallSide("source", row.Source, row.SourceName),
							blazar.Icon().
								Class("filter-icon").
								Icon(func() string {
									if c.filter.PhoneNumber == row.Source {
										return tabler.IconFilledFilter
									} else {
										return tabler.IconOutlineFilter
									}
								}()).
								Style("cursor", "pointer").
								Style("color", func() string {
									if c.filter.PhoneNumber == row.Source {
										return "var(--blazar-theme-primary)"
									}
									return "lightgray"
								}()).
								DataSet("phone-number", row.Source).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									phoneNumber := e.Target.Call("closest", ".filter-icon").Get("dataset").Get("phoneNumber").String()

									if c.filter.PhoneNumber == phoneNumber {
										c.filter.PhoneNumber = ""
									} else {
										c.filter.PhoneNumber = phoneNumber
									}
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
						)
				},
			},
			{
				Name: "Destination",
				Value: func(row CallRow) any {
					if row.Destination == "" {
						return ""
					}
					return app.Div().
						Class("call-side-container").
						Body(
							c.renderCallSide("destination", row.Destination, row.DestinationName),
							blazar.Icon().
								Class("filter-icon").
								Icon(func() string {
									if c.filter.PhoneNumber == row.Destination {
										return tabler.IconFilledFilter
									} else {
										return tabler.IconOutlineFilter
									}
								}()).
								Style("cursor", "pointer").
								Style("color", func() string {
									if c.filter.PhoneNumber == row.Destination {
										return "var(--blazar-theme-primary)"
									}
									return "lightgray"
								}()).
								DataSet("phone-number", row.Destination).
								OnClick(func(ctx app.Context, e htmlevent.MouseEvent) {
									phoneNumber := e.Target.Call("closest", ".filter-icon").Get("dataset").Get("phoneNumber").String()

									if c.filter.PhoneNumber == phoneNumber {
										c.filter.PhoneNumber = ""
									} else {
										c.filter.PhoneNumber = phoneNumber
									}
									ctx.SetState("filter", c.filter).Persist()

									c.filteredCallRows = c.filterCallRows(c.callRows)
									ctx.Update()
								}),
						)
				},
			},
			{
				Name: "Duration",
				Value: func(row CallRow) any {
					if row.Duration == 0 {
						return ""
					}
					return row.Duration.String()
				},
			},
			{
				Name: "Result",
				Value: func(row CallRow) any {
					item := ""
					if len(row.Sequence) > 0 {
						item = row.Sequence[len(row.Sequence)-1]
					}
					icon := sequenceItemToIcon(item)
					if icon == "" {
						return ""
					}

					return blazar.Icon().
						Class("result-" + item).
						Title(item).
						Icon(icon)
				},
			},
			{
				Name: "Result User",
				Value: func(row CallRow) any {
					return row.ResultUser
				},
			},
		}).
		Rows(callRows)
}

func formatPhoneNumber(phoneNumber string) string {
	if phoneNumber == "" {
		return ""
	}
	if strings.HasPrefix(phoneNumber, "+1") {
		localPortion := phoneNumber[2:]
		if len(localPortion) == 10 {
			return "(" + localPortion[:3] + ") " + localPortion[3:6] + "-" + localPortion[6:]
		}
	}
	return phoneNumber
}

func (c *CallsPage) buildCallRows(calls []*Call) []CallRow {
	rows := []CallRow{}
	for _, call := range calls {
		row := CallRow{
			Time:            call.Time,
			Source:          call.Source,
			SourceName:      call.SourceName,
			Destination:     call.Destination,
			DestinationName: call.DestinationName,
			Duration:        call.CallDuration,
		}
		if strings.Contains(call.EventName, "INCOMING") || call.EventName == "MSG_RECEIVED" {
			row.Direction = DirectionIn
		} else if strings.Contains(call.EventName, "OUTGOING") || call.EventName == "MSG_SENT" {
			row.Direction = DirectionOut
		}

		message := call.EventType == "EVENT_TYPE_MSG"
		ringGroup := false
		voicemail := false
		incomplete := false
		resultUser := ""
		for _, call := range call.Calls {
			if call.EventName == "RING_GROUP_VOICEMAIL_RECEIVED" {
				voicemail = true
				break
			}

			if call.RingGroup {
				ringGroup = true
			}
			if call.RingGroup {
				incomplete = true
			}

			if call.CallDuration > 0 {
				incomplete = false

				switch row.Direction {
				case DirectionIn:
					resultUser = call.DestinationName
				}
			}
		}
		if row.Direction == DirectionOut {
			resultUser = call.SourceName
			if call.CallDuration == 0 {
				incomplete = true
			}
		}
		if message && row.Direction == DirectionIn {
			resultUser = call.DestinationName
		}

		row.ResultUser = resultUser

		switch row.Direction {
		case DirectionIn:
			switch call.EventType {
			case "EVENT_TYPE_PBX_RING_GROUP":
				row.Sequence = append(row.Sequence, SequenceRingGroup)
			case "EVENT_TYPE_PBX_AUTO_ATTENDANT":
				row.Sequence = append(row.Sequence, SequenceAutoAttendant)
			case "EVENT_TYPE_CALL":
				row.Sequence = append(row.Sequence, SequencePhone)
			}

			if ringGroup {
				row.Sequence = append(row.Sequence, SequenceRingGroup)
			}

			if voicemail {
				row.Sequence = append(row.Sequence, SequenceVoicemail)
			} else if incomplete {
				row.Sequence = append(row.Sequence, SequenceIncomplete)
			} else if call.CallDuration > 0 {
				switch call.EventType {
				case "EVENT_TYPE_PBX_RING_GROUP":
					if len(call.Calls) > 0 {
						row.Sequence = append(row.Sequence, SequenceConnected)
					}
				case "EVENT_TYPE_PBX_AUTO_ATTENDANT":
					if len(call.Calls) > 0 {
						row.Sequence = append(row.Sequence, SequenceConnected)
					}
				case "EVENT_TYPE_CALL":
					row.Sequence = append(row.Sequence, SequenceConnected)
				}
			}

			if message {
				row.Sequence = append(row.Sequence, SequenceMessage)
			}
		case DirectionOut:
			if message {
				row.Sequence = append(row.Sequence, SequenceMessage)
			} else if voicemail {
				row.Sequence = append(row.Sequence, SequenceVoicemail)
			} else if incomplete {
				row.Sequence = append(row.Sequence, SequenceIncomplete)
			} else if call.CallDuration > 0 {
				row.Sequence = append(row.Sequence, SequenceConnected)
			}
		}

		rows = append(rows, row)
	}
	return rows
}

func (c *CallsPage) renderCalls(calls []*Call) app.UI {
	return app.Range(calls).Slice(func(i int) app.UI {
		call := calls[i]

		return app.Div().
			Class("call").
			Style("display", "flex").
			Body(
				app.Span().
					Class("time").
					Text(call.Time.Format(time.RFC3339Nano)),
				app.Div().
					Style("padding-left", "1em").
					Body(
						app.If(call.EventType != "", func() app.UI {
							icon := ""
							switch call.EventType {
							case "EVENT_TYPE_PBX_RING_GROUP":
								icon = tabler.IconOutlineUsers
							case "EVENT_TYPE_PBX_AUTO_ATTENDANT":
								icon = tabler.IconOutlineSitemap
							case "EVENT_TYPE_CALL":
								icon = tabler.IconFilledPhone
							case "EVENT_TYPE_MSG":
								icon = tabler.IconOutlineMessage
							}

							if icon == "" {
								return app.Span().Text(call.EventType)
							}
							return blazar.Icon().
								Icon(icon)
						}),
						app.Text(" | "),
						app.If(call.EventName != "", func() app.UI {
							icon := ""
							switch call.EventName {
							case "AUTO_ATTENDANT_INCOMING_CALL_RECEIVED":
								icon = tabler.IconOutlineArrowLeft
							case "INCOMING_CALL_RECEIVED":
								icon = tabler.IconOutlineArrowLeft
							case "MSG_RECEIVED":
								icon = tabler.IconOutlineArrowLeft
							case "MSG_SENT":
								icon = tabler.IconOutlineArrowRight
							case "OUTGOING_CALL_MADE":
								icon = tabler.IconOutlineArrowRight
							case "RING_GROUP_INCOMING_CALL_RECEIVED":
								icon = tabler.IconOutlineArrowLeft
							case "RING_GROUP_OUTGOING_CALL_MADE":
								icon = tabler.IconOutlineArrowRight
							case "RING_GROUP_VOICEMAIL_RECEIVED":
								icon = tabler.IconOutlineRecordMail
							}

							if icon == "" {
								return app.Span().Text(call.EventName)
							}
							return blazar.Icon().
								Icon(icon)
						}),
						app.If(call.EventStatus != "", func() app.UI {
							if call.EventStatus == "SUCCEEDED" {
								return nil
							}
							return app.Text(" (" + call.EventStatus + ")")
						}),
						app.Span().Text(" "),
						app.If(call.Source != "" || call.SourceName != "", func() app.UI {
							text := call.Source
							if call.SourceName != "" {
								text += " (" + call.SourceName + ")"
							}
							return app.Span().Text(text)
						}),
						app.If(call.Source != "" || call.SourceName != "", func() app.UI {
							return app.Span().Text(" -> ")
						}),
						app.Span().Text(call.Destination+" ("+call.DestinationType+") + "+call.DestinationName),
						app.Br(),
						app.If(call.CallDuration > 0, func() app.UI {
							return app.Span().Text("Duration: " + call.CallDuration.String())
						}),
						app.If(call.CallDuration > 0, func() app.UI {
							return app.Span().Text(" ")
						}),
						app.If(call.RingDuration > 0, func() app.UI {
							return app.Span().Text("Ring Duration: " + call.RingDuration.String())
						}),
						app.If(call.RingDuration > 0, func() app.UI {
							return app.Span().Text(" ")
						}),
						app.If(len(call.Calls) > 0, func() app.UI {
							return app.Div().
								Style("padding-left", "1em").
								Body(
									c.renderCalls(call.Calls),
								)
						}),
					),
			)
	})
}

func (c *CallsPage) login(ctx app.Context) error {
	slog.InfoContext(ctx.Context, "CallPage: login.")
	c.loggingIn = true
	defer func() {
		c.loggingIn = false
		ctx.Update()
	}()

	if c.authClient == nil {
		slog.ErrorContext(ctx.Context, "CallPage: login: authClient is nil")
		return fmt.Errorf("authClient is nil")
	}

	err := c.authClient.RequestAccessToken()
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: login: error requesting access token.", "err", err)
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	err = c.setTokenAsPartOfLoginWorkflow(ctx, c.authClient.Token())
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: login: error logging in with token.", "err", err)
		return fmt.Errorf("failed to login with token: %w", err)
	}

	return nil
}

func (c *CallsPage) setTokenAsPartOfLoginWorkflow(ctx app.Context, token oauth2.Token) error {
	tokenSource := oauth2.StaticTokenSource(&token)
	slog.InfoContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: tokenSource.", "tokenSource", tokenSource)

	googleOauthClient, err := googleoauth2.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: error creating google oauth client.", "err", err)
		return fmt.Errorf("failed to create google oauth client: %w", err)
	}

	slog.InfoContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: googleOauthClient created.", "googleOauthClient", googleOauthClient)
	userInfo, err := googleOauthClient.Userinfo.Get().Do()
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: could not get user info.", "err", err)

		if retrieveErr, ok := errors.AsType[*googleapi.Error](err); ok {
			slog.ErrorContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: oauth2 error.", "Code", retrieveErr.Code)
			if retrieveErr.Code == 401 {
				c.logout(ctx)
			}
		}

		return fmt.Errorf("failed to get user info: %w", err)
	}
	slog.InfoContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: userInfo retrieved.", "userInfo", userInfo)
	c.userInfo = userInfo
	ctx.SetState("user-info", c.userInfo).Persist()

	slog.InfoContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: Sending snackbar message.", "Email", c.userInfo.Email)
	blazar.SendSnackbarMessage(ctx, "snackbar-message", blazar.SnackbarMessage().Closable(true).Text("You are logged in as "+c.userInfo.Email))

	adminClient, err := admin.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: could not create admin client.", "err", err)
		return fmt.Errorf("failed to create admin client: %w", err)
	}
	slog.InfoContext(ctx.Context, "CallPage: setTokenAsPartOfLoginWorkflow: adminClient created.", "adminClient", adminClient)
	c.adminClient = adminClient

	return nil
}

// tokenIsExpired returns true if the token is expired.
func (c *CallsPage) tokenIsExpired(ctx app.Context) bool {
	if c.userToken == nil {
		return true
	}
	if c.userToken.AccessToken != "" {
		if c.userToken.Expiry.Before(time.Now()) {
			return true
		}
	}

	tokenInfo, err := c.authClient.TokenInfo(ctx.Context, c.userToken.AccessToken)
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: tokenIsExpired: error", "err", err)
		return true
	}

	if tokenInfo.ExpiresIn < 300*time.Second {
		return true
	}

	return false
}

func (c *CallsPage) refreshToken(ctx app.Context) error {
	slog.InfoContext(ctx.Context, "CallPage: refreshToken")
	if c.authClient == nil {
		slog.ErrorContext(ctx.Context, "CallPage: refreshToken: authClient is nil")
		return fmt.Errorf("authClient is nil")
	}
	if c.userInfo == nil {
		slog.ErrorContext(ctx.Context, "CallPage: refreshToken: userInfo is nil")
		return fmt.Errorf("userInfo is nil")
	}

	err := c.authClient.RequestAccessToken(
		googleoauthjs.WithOverridablePrompt(googleoauthjs.PromptEmptyString),
		googleoauthjs.WithOverridableLoginHint(c.userInfo.Id),
	)
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: refreshToken: error", "err", err)
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	err = c.setTokenAsPartOfLoginWorkflow(ctx, c.authClient.Token())
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: refreshToken: error", "err", err)
		return fmt.Errorf("failed to login with token: %w", err)
	}

	return nil
}

func (c *CallsPage) logout(ctx app.Context) {
	slog.InfoContext(ctx.Context, "CallPage: logout")
	c.userInfo = nil
	ctx.SetState("user-info", nil).Persist()

	c.userToken = nil
	ctx.SetState("user-token", nil).Persist()

	c.rawActivities = nil
	c.calls = nil
	c.callRows = nil
	c.filteredCallRows = nil

	blazar.SendSnackbarMessage(ctx, "snackbar-message", blazar.SnackbarMessage().Closable(true).Text("You have been logged out."))
}

func (c *CallsPage) search(ctx app.Context) {
	slog.InfoContext(ctx.Context, "CallPage: search")

	if c.adminClient == nil {
		slog.InfoContext(ctx.Context, "CallPage: search: adminClient is nil")
		return
	}

	var startTime *time.Time
	var endTime *time.Time

	switch c.configuration.Timeframe {
	case "past-24-hours":
		t := time.Now().Add(-24 * time.Hour)
		startTime = &t
	case "past-7-days":
		t := time.Now().Add(-7 * 24 * time.Hour)
		startTime = &t
	case "past-30-days":
		t := time.Now().Add(-30 * 24 * time.Hour)
		startTime = &t
	case "past-90-days":
		t := time.Now().Add(-90 * 24 * time.Hour)
		startTime = &t
	case "past-180-days":
		t := time.Now().Add(-180 * 24 * time.Hour)
		startTime = &t
	case "custom":
		if c.configuration.StartTimeString != "" {
			t, err := time.Parse(time.DateOnly, c.configuration.StartTimeString)
			if err != nil {
				slog.ErrorContext(ctx.Context, "CallPage: search: error", "err", err)
				return
			}
			startTime = &t
		}
		if c.configuration.EndTimeString != "" {
			t, err := time.Parse(time.DateOnly, c.configuration.EndTimeString)
			if err != nil {
				slog.ErrorContext(ctx.Context, "CallPage: search: error", "err", err)
				return
			}
			endTime = &t
		}
	default:
		slog.ErrorContext(ctx.Context, "CallPage: search: unknown timeframe", "timeframe", c.configuration.Timeframe)
		return
	}

	ctx.Dispatch(func(ctx app.Context) {
		c.searching = true
	})

	ctx.Async(func() {
		if c.tokenIsExpired(ctx) {
			slog.InfoContext(ctx.Context, "CallPage: search: token is expired")
			err := c.refreshToken(ctx)
			if err != nil {
				slog.ErrorContext(ctx.Context, "CallPage: search: error", "err", err)
				ctx.Dispatch(func(ctx app.Context) {
					c.searching = false
				})
				blazar.SendSnackbarMessage(ctx, "snackbar-message", blazar.SnackbarMessage().Closable(true).Bad().Text("Error searching: "+err.Error()))
				return
			}
		}

		call := c.adminClient.Activities.List("all", "voice")
		if startTime != nil {
			call = call.StartTime(startTime.Format(time.RFC3339Nano))
		}
		if endTime != nil {
			call = call.EndTime(endTime.Format(time.RFC3339Nano))
		}

		rawActivities := []*admin.Activity{}
		for {
			activities, err := call.Do()
			if err != nil {
				var retrieveErr *googleapi.Error
				if errors.As(err, &retrieveErr) {
					slog.ErrorContext(ctx.Context, "CallPage: search: oauth2 error", "ErrorCode", retrieveErr.Code)
				}

				slog.ErrorContext(ctx.Context, "CallPage: search: error", "err", err)
				ctx.Dispatch(func(ctx app.Context) {
					c.searching = false
					ctx.Update()
				})
				blazar.SendSnackbarMessage(ctx, "snackbar-message", blazar.SnackbarMessage().Closable(true).Bad().Text("Error searching: "+err.Error()))
				return
			}
			slog.InfoContext(ctx.Context, "CallPage: search: activities", "activities", len(activities.Items))

			rawActivities = append(rawActivities, activities.Items...)

			if activities.NextPageToken == "" {
				break
			}
			call = call.PageToken(activities.NextPageToken)
		}

		ctx.Dispatch(func(ctx app.Context) {
			c.searching = false
			c.searched = true
			c.rawActivities = rawActivities
			c.calls = parseCalls(ctx.Context, rawActivities)
			c.callRows = c.buildCallRows(c.calls)
			c.filteredCallRows = c.filterCallRows(c.callRows)
		})
	})
}

type Call struct {
	Time            time.Time     // The time of the call.
	Source          string        // The source phone number.
	SourceName      string        // The name of the source.
	Destination     string        // The destination phone number.
	DestinationType string        // One of: auto-attendant, ring-group, phone.
	DestinationName string        // The name of the destination.
	RingDuration    time.Duration // The duration of the ring.
	CallDuration    time.Duration // The duration of the call.
	EventType       string        // The type of the event.
	EventName       string        // The name of the event.
	EventStatus     string        // The status of the event.
	RingGroup       bool          // True if the call is a ring group call.
	Calls           []*Call       // Only when DestinationType is auto-attendant or ring-group.
}

func parseCalls(ctx context.Context, rawActivities []*admin.Activity) []*Call {
	calls := []*Call{}

	rawActivitiesCopy := make([]*admin.Activity, len(rawActivities))
	copy(rawActivitiesCopy, rawActivities)
	rawActivities = rawActivitiesCopy

	slices.SortFunc(rawActivities, func(a, b *admin.Activity) int {
		return strings.Compare(a.Id.Time, b.Id.Time)
	})

	for _, rawActivity := range rawActivities {
		for _, rawEvent := range rawActivity.Events {
			parameters := map[string]*admin.ActivityEventsParameters{}
			for _, parameter := range rawEvent.Parameters {
				parameters[parameter.Name] = parameter
			}

			call := &Call{
				EventType:   rawEvent.Type,
				EventName:   rawEvent.Name,
				EventStatus: rawEvent.Status.EventStatus,
			}
			if parameters["PARAM_SOURCE"] == nil {
				continue
			}
			if parameters["PARAM_DESTINATION"] == nil {
				continue
			}
			t, err := time.Parse(time.RFC3339Nano, rawActivity.Id.Time)
			if err != nil {
				slog.ErrorContext(ctx, "CallPage: parseCalls: error", "err", err)
				continue
			}

			// Skip stats for now.  TODO: CONSIDER ROLLING THESE INTO THE OVERALL CALL.
			if rawEvent.Type == "EVENT_TYPE_NETWORK_STATISTICS_CLIENT" {
				continue
			}
			// Skip the ring group metadata; we only care about the actual calls that were made.
			if rawEvent.Type == "EVENT_TYPE_PBX_RING_GROUP" && rawEvent.Name == "RING_GROUP_OUTGOING_CALL_MADE" {
				continue
			}
			// Skip the ring group summary record.
			if rawEvent.Type == "EVENT_TYPE_PBX_RING_GROUP" && rawEvent.Name == "RING_GROUP_INCOMING_CALL_RECEIVED" {
				continue
			}
			// Skip the auto attendant summary record.
			if rawEvent.Type == "EVENT_TYPE_PBX_AUTO_ATTENDANT" && rawEvent.Name == "AUTO_ATTENDANT_OUTGOING_CALL_MADE" {
				continue
			}

			call.Time = t
			call.Source = parameters["PARAM_SOURCE"].Value
			call.Destination = parameters["PARAM_DESTINATION"].Value

			if parameters["PARAM_RING_DURATION"] != nil && parameters["PARAM_RING_DURATION"].IntValue > 0 {
				call.RingDuration = time.Duration(time.Duration(parameters["PARAM_RING_DURATION"].IntValue) * time.Millisecond)
			}
			if parameters["PARAM_DURATION"] != nil && parameters["PARAM_DURATION"].IntValue > 0 {
				call.CallDuration = time.Duration(time.Duration(parameters["PARAM_DURATION"].IntValue) * time.Millisecond)
			}
			if parameters["PARAM_DISTRIBUTION_ID"] != nil && parameters["PARAM_DISTRIBUTION_ID"].Value != "" {
				call.RingGroup = true
			}
			if strings.HasPrefix(rawActivity.ResourceDetails[0].Id, "phone_number/") {
				call.DestinationType = "phone"
				switch rawEvent.Name {
				case "INCOMING_CALL_RECEIVED", "MSG_RECEIVED":
					call.DestinationName = rawActivity.Actor.Email
				case "OUTGOING_CALL_MADE", "MSG_SENT":
					call.SourceName = rawActivity.Actor.Email
				default:
					call.DestinationName = rawActivity.ResourceDetails[0].Title
				}
			} else if strings.HasPrefix(rawActivity.ResourceDetails[0].Id, "auto_attendant/") {
				call.DestinationType = "auto-attendant"
				call.DestinationName = rawActivity.ResourceDetails[0].Title
			} else if strings.HasPrefix(rawActivity.ResourceDetails[0].Id, "ring_group/") {
				call.DestinationType = "ring-group"
				call.DestinationName = rawActivity.ResourceDetails[0].Title
			}
			if call.DestinationName == call.Destination {
				call.DestinationName = ""
			}
			calls = append(calls, call)
		}
	}

	aggregatedCalls := aggregateCalls(calls)

	slices.Reverse(aggregatedCalls)

	return aggregatedCalls
}

func aggregateCalls(calls []*Call) []*Call {
	aggregatedCalls := []*Call{}
	for len(calls) > 0 {
		call := calls[0]
		calls = calls[1:]

		if call.DestinationType == "auto-attendant" || call.DestinationType == "ring-group" {
			for c := 0; c < len(calls); c++ {
				if calls[c].Source == call.Source && calls[c].Time.Before(call.Time.Add(call.CallDuration)) {
					call.Calls = append(call.Calls, calls[c])
					calls = append(calls[:c], calls[c+1:]...)
					c--
				}
			}
			call.Calls = aggregateCalls(call.Calls)
			for c := range call.Calls {
				call.Calls[c].Source = ""
				call.Calls[c].SourceName = ""
			}
		}
		aggregatedCalls = append(aggregatedCalls, call)
	}
	return aggregatedCalls
}

func (c *CallsPage) filterCallRows(callRows []CallRow) []CallRow {
	filteredCallRows := []CallRow{}

	var allowedFinalStates []string
	{
		if c.filter.ResultAutoAttendant {
			allowedFinalStates = append(allowedFinalStates, SequenceAutoAttendant)
		}
		if c.filter.ResultRingGroup {
			allowedFinalStates = append(allowedFinalStates, SequenceRingGroup)
		}
		if c.filter.ResultPhone {
			allowedFinalStates = append(allowedFinalStates, SequencePhone)
		}
		if c.filter.ResultMessage {
			allowedFinalStates = append(allowedFinalStates, SequenceMessage)
		}
		if c.filter.ResultVoicemail {
			allowedFinalStates = append(allowedFinalStates, SequenceVoicemail)
		}
		if c.filter.ResultIncomplete {
			allowedFinalStates = append(allowedFinalStates, SequenceIncomplete)
		}
		if c.filter.ResultConnected {
			allowedFinalStates = append(allowedFinalStates, SequenceConnected)
		}
	}

	for _, callRow := range callRows {
		if c.filter.Inbound && callRow.Direction != DirectionIn {
			continue
		}
		if c.filter.Outbound && callRow.Direction != DirectionOut {
			continue
		}

		if len(allowedFinalStates) > 0 {
			if len(callRow.Sequence) == 0 {
				continue
			}
			if !slices.Contains(allowedFinalStates, callRow.Sequence[len(callRow.Sequence)-1]) {
				continue
			}
		}

		if c.filter.PhoneNumber != "" {
			if callRow.Source != c.filter.PhoneNumber && callRow.Destination != c.filter.PhoneNumber {
				continue
			}
		}

		filteredCallRows = append(filteredCallRows, callRow)
	}
	return filteredCallRows
}
