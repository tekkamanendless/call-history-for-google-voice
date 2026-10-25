# call-history-for-google-voice
A web UI for Google Voice to show call history.

Once you grant access to your Google Workspace account, you'll get a simple table of call records.
The calls will be aggregated into logical calls, with some metadata around auto attendants, ring groups, and users.

# Published Online
This tool is published online here:
https://call-history-for-google-voice.github.tekkamanendless.net

# Motivation
Google Voice (enterprise) does not have a good call dashboard, especially with regard to call history.

In addition, when a member of ring group _misses_ a call, that call is _not_ recorded in that user's Google Voice call history.
This makes it extremely difficult and annoying to see if a call was missed.

At my company, we have an auto attendant that rings to a "front desk" ring group, and if no one answers, it goes to voicemail.

Google Voice doesn't really provide any visibility into who calls the main line, if they transferred to the front desk, if they hung up early before connecting, or if the call resulted in a voicemail.
All it provides are audit logs, which are difficult to read, and they are not aggregated by logical call.
To read the audit logs, you have to mentally construct a logical phone call, which is really only possible if you know the exact setup of the phone system.

This tool tries to solve that problem by making it easy to see the calls that came in (or went out) and what the ultimate result was.

# Development

## GCP Configuration
1. Enable the Admin SDK: https://console.developers.google.com/apis/api/admin.googleapis.com/overview
