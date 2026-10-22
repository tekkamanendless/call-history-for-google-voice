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
	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"golang.org/x/oauth2"
	admin "google.golang.org/api/admin/reports/v1"
	"google.golang.org/api/googleapi"
	googleoauth2 "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

type CallsPage struct {
	app.Compo

	startTimeString string
	endTimeString   string
	maxResults      uint

	errorMessage string

	authClient    app.Value
	userInfo      *googleoauth2.Userinfo
	adminClient   *admin.Service
	rawActivities []*admin.Activity
	calls         []*Call
}

func (c *CallsPage) OnMount(ctx app.Context) {
	slog.InfoContext(ctx.Context, "CallPage: OnMount")

	c.maxResults = 0
	c.startTimeString = time.Now().Add(-1 * time.Hour * 24).Format(time.DateOnly)
	//c.endTime = time.Now()

	{
		var token oauth2.Token
		ctx.GetState("user-token", &token)
		if token.AccessToken != "" {
			slog.InfoContext(ctx.Context, "CallPage: OnMount: callback: saved token", "token", token)
			c.loginWithToken(ctx, token)
		}
	}

	config := map[string]any{
		"client_id": "531363734726-vvb6f23uehpuk2joljh46uj5igk745qv.apps.googleusercontent.com",
		"scope": strings.Join([]string{
			"profile",
			"https://www.googleapis.com/auth/admin.reports.audit.readonly",
		}, " "),
		"ux_mode": "popup",
		"callback": app.FuncOf(func(this app.Value, args []app.Value) any {
			ctx.Async(func() {
				slog.InfoContext(ctx.Context, "CallPage: OnMount: callback", "args", len(args))
				if len(args) > 0 {
					info := args[0]
					slog.InfoContext(ctx.Context, "CallPage: OnMount: callback: info", "JSON", app.Window().Get("JSON").Call("stringify", info))

					token := oauth2.Token{
						AccessToken: info.Get("access_token").String(),
						ExpiresIn:   int64(info.Get("expires_in").Int()),
						TokenType:   info.Get("token_type").String(),
					}

					c.loginWithToken(ctx, token)

					ctx.SetState("user-info", c.userInfo).Persist()
					ctx.SetState("user-token", token).Persist()

					ctx.Update()
				}
			})
			return nil
		}),
	}
	c.authClient = app.Window().Get("google").Get("accounts").Get("oauth2").Call("initTokenClient", config)

	/*
		app.Window().Set("test_function", app.FuncOf(func(this app.Value, args []app.Value) any {
			slog.InfoContext(ctx.Context, "CallPage: OnMount: test_function", "args", len(args))
			return nil
		}))
	*/

	ctx.Update()
}

func (c *CallsPage) OnNav(ctx app.Context) {
	slog.InfoContext(ctx.Context, "CallPage: OnNav")
}

func (c *CallsPage) Render() app.UI {
	return blazar.Page().
		Body(
			app.If(c.userInfo == nil, func() app.UI {
				return blazar.Button().
					Label("Sign in with Google").
					On("click", func(ctx app.Context, e app.Event) {
						c.authClient.Call("requestAccessToken")
					})
			}),
			app.If(c.userInfo != nil, func() app.UI {
				/*
					return app.Div().
						Body(
							app.Img().Src(c.userInfo.Picture).Width(32).Height(32),
							app.Span().Text(c.userInfo.Name),
						)
				*/
				return blazar.Button().
					Label("Log out as "+c.userInfo.Name).
					On("click", func(ctx app.Context, e app.Event) {
						c.logout(ctx)
						ctx.Update()
					})
			}),
			/*
				app.If(c.authClient != nil, func() app.UI {
					return app.Div().
						ID("g_id_onload").
						DataSet("client_id", "531363734726-vvb6f23uehpuk2joljh46uj5igk745qv.apps.googleusercontent.com").
						DataSet("context", "signin").
						DataSet("ux_mode", "popup").
						DataSet("auto_prompt", "false").
						DataSet("callback", "test_function")
				}),
				app.If(c.authClient != nil, func() app.UI {
					return app.Div().
						Class("g_id_signin").
						DataSet("type", "standard").
						DataSet("shape", "rectangular").
						DataSet("size", "large").
						DataSet("theme", "outline").
						DataSet("text", "signin_with").
						DataSet("callback", "test_function")
				}),
			*/
			blazar.Collapse().
				Label("Configuration").
				Open(true).
				Body(
					blazar.Form().
						Body(
							blazar.Input[string]().
								Label("Start Time").
								Clearable(true).
								Type("date").
								Bind(&c.startTimeString),
							blazar.Input[string]().
								Label("End Time").
								Clearable(true).
								Type("date").
								Bind(&c.endTimeString),
							blazar.Input[uint]().
								Label("Max Results").
								Bind(&c.maxResults),
						).
						Action(blazar.FormAction{
							Name:     "Search",
							Function: c.search,
						}),
				),
			app.If(c.errorMessage != "", func() app.UI {
				return blazar.StatusBar().
					Bad().
					Text(c.errorMessage)
			}),
			app.If(len(c.calls) > 0, func() app.UI {
				return app.Div().
					Style("font-size", "0.8em").
					Body(
						app.H2().Text("Calls"),
						renderCalls(c.calls),
					)
			}),
			app.If(len(c.rawActivities) > 0, func() app.UI {
				return app.Div().
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
					)
			}),
		)
}

func renderCalls(calls []*Call) app.UI {
	return app.Range(calls).Slice(func(i int) app.UI {
		call := calls[i]

		return app.Div().
			Class("call").
			Style("display", "flex").
			Body(
				app.Span().
					Class("time").
					Text(call.Time.Format(time.RFC3339)),
				app.Div().
					Style("padding-left", "1em").
					Body(
						app.If(call.EventType != "", func() app.UI {
							icon := ""
							switch call.EventType {
							case "EVENT_TYPE_PBX_RING_GROUP":
								icon = "user-group"
							case "EVENT_TYPE_PBX_AUTO_ATTENDANT":
								icon = "tty"
							case "EVENT_TYPE_CALL":
								icon = "phone"
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
								icon = "arrow-left"
							case "INCOMING_CALL_RECEIVED":
								icon = "arrow-left"
							case "OUTGOING_CALL_MADE":
								icon = "arrow-right"
							case "RING_GROUP_VOICEMAIL_RECEIVED":
								icon = "voicemail"
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
									renderCalls(call.Calls),
								)
						}),
					),
			)
	})
}

func (c *CallsPage) loginWithToken(ctx app.Context, token oauth2.Token) {
	tokenSource := oauth2.StaticTokenSource(&token)
	//oauthClient := oauth2.NewClient(ctx, tokenSource)
	slog.InfoContext(ctx.Context, "CallPage: loginWithToken: tokenSource", "tokenSource", tokenSource)

	googleOauthClient, err := googleoauth2.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: loginWithToken: error", "err", err)
		return
	}

	slog.InfoContext(ctx.Context, "CallPage: loginWithToken: googleOauthClient", "googleOauthClient", googleOauthClient)
	userInfo, err := googleOauthClient.Userinfo.Get().Do()
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: loginWithToken: could not get user info", "err", err)

		var retrieveErr *googleapi.Error
		if errors.As(err, &retrieveErr) {
			slog.ErrorContext(ctx.Context, "CallPage: loginWithToken: oauth2 error", "ErrorCode", retrieveErr.Code)
			if retrieveErr.Code == 401 {
				c.logout(ctx)
			}
		}

		return
	}
	slog.InfoContext(ctx.Context, "CallPage: loginWithToken: userInfo", "userInfo", userInfo)
	c.userInfo = userInfo

	adminClient, err := admin.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		slog.ErrorContext(ctx.Context, "CallPage: loginWithToken: could not create admin client", "err", err)
		return
	}
	slog.InfoContext(ctx.Context, "CallPage: loginWithToken: adminClient", "adminClient", adminClient)
	c.adminClient = adminClient
}

func (c *CallsPage) logout(ctx app.Context) {
	slog.InfoContext(ctx.Context, "CallPage: logout")
	c.userInfo = nil
	ctx.SetState("user-info", nil).Persist()
	ctx.SetState("user-token", nil).Persist()
}

func (c *CallsPage) search(ctx app.Context) {
	slog.InfoContext(ctx.Context, "CallPage: search")

	if c.adminClient == nil {
		slog.InfoContext(ctx.Context, "CallPage: search: adminClient is nil")
		return
	}

	ctx.Async(func() {
		call := c.adminClient.Activities.List("all", "voice")
		if c.startTimeString != "" {
			startTime, err := time.Parse(time.DateOnly, c.startTimeString)
			if err != nil {
				slog.ErrorContext(ctx.Context, "CallPage: search: error", "err", err)
				return
			}
			call = call.StartTime(startTime.Format(time.RFC3339))
		}
		if c.endTimeString != "" {
			endTime, err := time.Parse(time.DateOnly, c.endTimeString)
			if err != nil {
				slog.ErrorContext(ctx.Context, "CallPage: search: error", "err", err)
				return
			}
			call = call.EndTime(endTime.Format(time.RFC3339))
		}
		if c.maxResults > 0 {
			call = call.MaxResults(int64(c.maxResults))
		}

		rawActivities := []*admin.Activity{}
		for {
			activities, err := call.Do()
			if err != nil {
				slog.ErrorContext(ctx.Context, "CallPage: search: error", "err", err)
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
			c.rawActivities = rawActivities
			c.calls = parseCalls(ctx.Context, rawActivities)
			ctx.Update()
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
			t, err := time.Parse(time.RFC3339, rawActivity.Id.Time)
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
			if strings.HasPrefix(rawActivity.ResourceDetails[0].Id, "phone_number/") {
				call.DestinationType = "phone"
				call.DestinationName = rawActivity.ResourceDetails[0].Title
				if rawEvent.Name == "INCOMING_CALL_RECEIVED" {
					call.DestinationName = rawActivity.Actor.Email
				} else if rawEvent.Name == "OUTGOING_CALL_MADE" {
					call.SourceName = rawActivity.Actor.Email
				}
			} else if strings.HasPrefix(rawActivity.ResourceDetails[0].Id, "auto_attendant/") {
				call.DestinationType = "auto-attendant"
				call.DestinationName = rawActivity.ResourceDetails[0].Title
			} else if strings.HasPrefix(rawActivity.ResourceDetails[0].Id, "ring_group/") {
				call.DestinationType = "ring-group"
				call.DestinationName = rawActivity.ResourceDetails[0].Title
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
