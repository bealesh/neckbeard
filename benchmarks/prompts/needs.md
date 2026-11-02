Tell me what this application needs to run in production. I want the facts, each with the file and line that proves it:

- every process it runs and what role each plays (HTTP service, background worker, scheduled job), including the command that starts it;
- the port it listens on and the path a load balancer should use for health checks;
- every datastore it talks to and how it finds it;
- the environment variables and secrets it reads, and which of those are secrets;
- anything that runs at startup, such as migrations;
- scheduled or periodic work and its schedule.

Write the answer to PRODUCTION_NEEDS.md in this directory. Do not generate infrastructure and do not ask me questions; if something cannot be determined from the code, say so.
