# secompose
Docker Compose alias with the ability to inject secrets from external backends

# Usage

Use secompose or se-compose like your normal docker compose or docker-compose CLI command:

```bash
secompose -f mysecompose.yml up -d
```

secompose parses the compose file, reads secompose-backed secrets and rebuilds an in-memory/stdin compose file to be transparently handed over to `docker compose`

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
    environment: GITLAB_PYPI_TOKEN
```

The `x-secompose-*` key is parsed **first** by secompose (in this example, if it fails to read the secret from Vault, it fails over to the `environment` value)

> `docker compose` will still parse this compose file correctly, however it will simply ignore the secompose key and use the `file` or `environment` associated with the secret.

> While it is recommended to set a failover source, you may choose to skip the `environment` or `file` key if you are sure you and your coworkers use secompose and not just `docker compose`.

## Providers
The `x-secompose-*` suffix depends on your secret provider:
- Hashicorp Vault: `vault`
- AWS Secret Manager: `aws`
- Azure Key Vault: `azure`

Depending on the targetted provider, you will need `path`, or `key`, or both, to be able to read the secrets. Additional/optional provider-specific keys may be used too, please refer to the provider section.

### Vault
Key: `x-secompose-vault`

Reads secrets from a kv (key-value) engine in Hashicorp Vault. 
Access to the Vault server should be granted by environment variables where `secompose` runs: VAULT_TOKEN, VAULT_ADDR, and any other optional env var described by the Vault documentation (e.g. VAULT_CACERT for private/custom CA...)

The provider expects `path` and `key` keys for each secret:
- `path`: Slash-separated path to access the secret. The first fragment is the mount/engine where the secret is located, e.g. `infra-kv/platform/domains/example_com/certificates` would refer to the secret named "certificates" found in the *infra-kv* kv engine, at the *platform/domains/example_com* path.
- `key`: Name of the key in the secret which value will be read







