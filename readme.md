This is based on the demo feed generator by willdot found here https://tangled.org/willdot.net/feed-demo-go  


## ATProto feed demo

This is a demo example of how to create a Bluesky feed generator written in Go. You can read about how they work from some official documentation [here](https://docs.bsky.app/docs/starter-templates/custom-feeds)

### What is a feed generator?
A quick high level overview is that it's a server that consumes post events from a firehose and then stores them in a database if they meet the criteria of a feed. For example, this repo demo will store any post that contains the text #golang.

The server is then registered as a feed that belongs to a user. Other users can then look at that feed and see the posts that the feed contains. When that happens, a request is sent to the feed server to get a feed. The server will work out what posts to return and its response will be what's called a skeleton; a list of post IDs that can be hydrated for the users by an appview.

### Running the app

There are 2 parts to this repo:

1: The feed generator server
2: A small cli tool that is used to register the feed.

If doing this in local development on your machine I suggest using something like ngrok to get a public facing URL that Bluesky can use to call your locally running feed server. You will need to expose the port `443` as part of this as that's the port that is used as part of the server. For example `ngrok http http://localhost:443` which will give you a publicly accessable URL. This URL is what you will need to use in your `.env` file detailed below.

A few environment variables are required to run the app. Use the `example.env` file as a template and store your environment variables in a `.env` file.

* BSKY_HANDLE - Your own handle which will allow the feed register script to authenticate and register the feed for you
* BSKY_PASS - A password to authenticate - app passwords are recomended here!
* FEED_HOST_NAME - This is the URL of where the feed server is hosted for example "demo-feed.com" (This should not include the protocol)
* FEED_NAME - This is a unique name you are going to give your feed that will be stored as an RKey in your PDS as a record
* FEED_DISPLAY_NAME - This is the name you will give your feed that users will be able to see
* FEED_DESCRIPTION - This is a description of your feed that users will be able to see
* FEED_DID - This is the DID that will be used to register the record. Unless you know what you are doing it's best to use `did:web:` +  FEED_HOST_NAME (eg "did:web:demo-feed.com")
* ACCEPTS_INTERACTIONS - Set this to be true if you wish your feed to accepts interactions such as "show more" or "show less"

First you need to run the feed generator by building the application `go build -o demo-feed-generator ./cmd/feed-generator/main.go` and then running it `./demo-feed-generator`

Next you need to register the feed which can be done by running from the root of this repo `go run cmd/register-feed/main.go`

This should print out some JSON and part of that will be a field `validationStatus` which should have the value `valid` if successful.

You can then head to your profile on Bluesky, go to the feeds section and you should see your feed. There may not be any posts on it as it's not likely someone has posted a post with #golang since you started your server. However if you create a post with #golang you should see it in your feed.

### Contributing
This is a demo of how to build and run a simple feed generator in Go. There are lots more things that can be done to create feeds but that can be left to you. I have kept it simple but if you wish to contribute then feel free to fork and PR any improvements you think there can be.
