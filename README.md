# secompose
Docker Compose wrapper with the ability to inject secrets from external backends

# Usage

Use secompose (or se-compose) like your normal docker compose or docker-compose CLI command:

```bash
secompose -f mysecompose.yml up -d
```

secompose parses the compose file, reads secompose-backed secrets and runs the compose file as-is with secret values injected as environment variables attached to the command.

# secompose file reference
## secompose extension keys

The only difference with a classic compose.yml lies in its extension of the `secrets` root keyword, where possible choices (on top of native `file` and `environment`) now include new `x-secompose-<provider_name>` keys.

These keys are structured as such:

```yaml
secrets:
  gitlab_pypi_token:
    x-secompose-vault: 
      path: myengine/infra/dev/gitlab
      key: global_pypi_reader
      version: 3
    environment: GITLAB_PYPI_TOKEN
```

When it exists, the `x-secompose-*` key is parsed **first** by secompose. If it fails to read the secret from the provider (here, Vault), it fails over to the `environment` value

> The above compose file is valid for `docker compose`: it will still simply ignore the x-secompose-* keys and proceed with reading the secret from the `environment` source.

> The `environment` source is mandatory: secompose must be able to fail over it, and secompose assigns the secret value to the env variable name set as `environment`. The `file` source is not supported by secompose.

## Providers
The `x-secompose-*` suffix depends on your secret provider, for instance:
- Hashicorp Vault: `vault`
- AWS Secret Manager: `aws`
- Azure Key Vault: `azure`
- GCP Secret Manager: `gcp`

For all the providers, the following base fields are available:
- `key`: Either the name of the secret, or the key of the secret (when a secret is a dict like in Vault).
- `path` (optional): The path to access the secret, when the provider lays out a hierarchy (Vault, Infisical...)
- `version` (optional): The version of the secret to fetch, when the provider allows for it. Defaults to "latest".

### Vault
Key: `x-secompose-vault`

Reads a secret from a kv (key-value) engine in Hashicorp Vault. 
Access to the Vault server should be granted by environment variables where `secompose` runs: VAULT_TOKEN, VAULT_ADDR, and any other optional env var described by the Vault documentation (e.g. VAULT_CACERT for private/custom CA...)

Additional fields:
- `namespace` (optional): Name of the mounted secret engine where the secret is stored. Defaults to "kv" if not set.


### GCP Secret Manager
Key: `x-secompose-gcp`

Reads a secret from the Secret Manager of the GCP project.
Access to the secret must be granted through your gcloud CLI, as `secompose` reads your default credentials.

Additional fields:
- `namespace` (optional): GCP project where the secret is hosted. When not set, `secompose` tries to find a default GCP project through (by priority order): from the `GOOGLE_CLOUD_PROJECT` environment variable, from the default project attached to your GCP/gcloud credentials.








