# swagger-importer
Import swagger APIs to Azure API Management APIs using Azure Managed Identity and federated credentials

# how it works

The operator will fetch the app: <app-name> label from workloads and match them towards the application: <app-name> label in the APIs.

It will fetch the swagger.json files from the running workloads and patch them into the API resources.