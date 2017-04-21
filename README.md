# amalgemate

Host a RubyGems repository that does not itself hold any gems, but instead will aggregate the available gems from one or more upstream repositories.

Repositories are aggregated with priority ordering. Say two repositories are being proxied, and both of them have foogem-1.0.0. Whichever one was specified first when starting amalgemate will be served to users.

**Right now this more proof-of-concept than ready to use tool.**

## TODO:

 * Handle contextless gem requests (i.e foo-gem-1.0.0.gem was requested without the info being loaded first)
