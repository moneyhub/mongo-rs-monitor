# Mongo replicaSet monitor #

Monitor status of mongo replica sets and get notified about issues via PagerDuty and/or Slack.


# Configuration #
It accepts only one argument - the config file path.

If not specified it will try to read `./config/local.json`


## Config options:
 - `mongoUsr` (optional)
 - `mongoPwd` (optional)
 - `pagerdutyKey` (optional)
 - `slackWebhook`  (optional)
 - `replicaSets` (array, each element includes below parameters): 
     - `name` (optional, defaults to `members`)
     - `members` (required, string with comma-separated nodes)
     - `mongoUsr` (optional, takes precedence over global value)
     - `mongoPwd` (optional, takes precedence over global value)
     - `checkInterval` (optional, default: 10s)
     - `tls` (optional, default: false)

User auth is done against `admin` db

Mongo built-in read-only role `clusterMonitor` or similar that allows `replSetGetStatus` query should be granted to the user

## Example config file:

```json
{   
   "mongUsr":      "clusterMonitor",
   "mongoPwd":     "pass",
   "pagerdutyKey": "pagerduty_key",
   "slackWebhook": "https://webhook",
   "replicaSets":[
        {
            "name": "production mongo",
            "members": "mongod1,mongod2,mongod3",
            "tls": true,
            "checkInterval": 15
        },
        {
            "name": "production mongo config",
            "members": "mongoc,mongoc2,mongoc3",
            "mongUsr":      "clusterMonitorConfig",
            "mongoPwd":     "passConfig"
        }
    ]
}
```