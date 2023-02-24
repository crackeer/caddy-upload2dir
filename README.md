## Requirement
- [Go Installed](https://golang.org/doc/install)

## Install
refer to [Extend Caddy](https://caddyserver.com/docs/extending-caddy)
1. **Install [xcaddy](https://github.com/caddyserver/xcaddy)**

```sh
go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
```

2. **Build A New Caddy Binary**

```sh
xcaddy build master --with=github.com/crackeer/caddy-upload2dir
```

3. **copy new template.html**

here is the [template.html](https://github.com/crackeer/caddy-upload2dir/blob/main/template.html)


## Example:caddy.json

```json
{
    "apps": {
        "http": {
            "servers": {
                "static": {
                    "idle_timeout": 30000000000,
                    "listen": [
                        "0.0.0.0:80"
                    ],
                    "max_header_bytes": 10240000,
                    "read_header_timeout": 10000000000,
                    "routes": [
                        {
                            "match" : [
                                {
                                    "method" : ["POST", "PUT", "DELETE"]
                                }
                            ],
                            "handle" : [
                                {
                                    "handler" : "upload2dir",
                                    "file_server_root" : "/your/file/dir",
                                    "max_filesize_int" : 99999999999999,
                                    "max_form_buffer_int" : 9999999999999,
                                    "user_config" : [
                                        "token:user_name:create_dir/delete_file/put_file"
                                    ],
                                    "user_token_cookie_key" : "user-token-key"
                                }
                            ],
                            "terminal" : true
                        },
                        {
                            "handle": [
                                {
                                    "handler": "file_server",
                                    "root": "/your/file/dir",
                                    "browse": {
                                        "template_file": "/new/template.html"
                                    },
                                    "index_names" : [""]
                                }
                            ]
                        }
                    ]
                }
            }
        }
    }
}
```

#### user_config
`token`:`username`:`create_dir`/`put_file`/`delete_file`
There are three actions you can config in user_config
- create_dir
- put_file
- delete_file


