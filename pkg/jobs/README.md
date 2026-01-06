
Jobs are simple commands that can be run in our stage and/or production environments.  

To execute them, they must be defined in ./deployments/jobs.yaml and ./deployments/deployment.yaml

If a job needs to re-run, you must update the name ./deployments/jobs.yaml.

./cmd/jobs/main.go is utilized to run them