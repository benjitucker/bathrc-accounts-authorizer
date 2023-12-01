package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/auth0/go-jwt-middleware/v2/jwks"
	"github.com/auth0/go-jwt-middleware/v2/validator"
)

var (
	logger log.Logger
)

// Help function to generate an IAM policy
func generatePolicy(principalId, effect, resource string) events.APIGatewayCustomAuthorizerResponse {
	authResponse := events.APIGatewayCustomAuthorizerResponse{PrincipalID: principalId}

	if effect != "" && resource != "" {
		authResponse.PolicyDocument = events.APIGatewayCustomAuthorizerPolicy{
			Version: "2012-10-17",
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   effect,
					Resource: []string{resource},
				},
			},
		}
	}

	// Optional output with custom properties of the String, Number or Boolean type.
	authResponse.Context = map[string]interface{}{
		"stringKey":  "stringval",
		"numberKey":  123,
		"booleanKey": true,
	}
	return authResponse
}

func toString(thing any) string {
	m, err := json.Marshal(thing)
	if err == nil {
		return err.Error()
	}
	return string(m)
}

// CustomClaims contains custom data we want from the token.
type CustomClaims struct {
	Scope string `json:"scope"`
}

// Validate does nothing for this example, but we need
// it to satisfy validator.CustomClaims interface.
func (c CustomClaims) Validate(ctx context.Context) error {
	return nil
}

func HandleRequest(ctx context.Context, event events.APIGatewayCustomAuthorizerRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	logger := log.With(logger, "method", "HandleRequest")
	_ = level.Debug(logger).Log("event", toString(event))

	token := event.AuthorizationToken

	// Setup the Auth0 Domain to Authenticate
	issuerURL, err := url.Parse("https://" + os.Getenv("AUTH0_DOMAIN") + "/")
	if err != nil {
		_ = level.Error(logger).Log("msg", "Failed to parse the issuer url", "err", err)
	}

	// Configure the Caching Provider for the validator
	provider := jwks.NewCachingProvider(issuerURL, 5*time.Minute)

	// Configure the jwtValidator using the Audience
	jwtValidator, err := validator.New(
		provider.KeyFunc,
		validator.RS256,
		issuerURL.String(),
		[]string{os.Getenv("AUTH0_AUDIENCE")},
		validator.WithCustomClaims(
			func() validator.CustomClaims {
				return &CustomClaims{}
			},
		),
		validator.WithAllowedClockSkew(time.Minute),
	)
	if err != nil {
		_ = level.Error(logger).Log("msg", "Failed to set up the jwt validator", "err", err)
	}

	// Validate the Token
	_, err = jwtValidator.ValidateToken(context.TODO(), token)
	if err != nil {
		//		return generatePolicy("user", "Deny", event.MethodArn), err
		return generatePolicy("user", "Deny", "*"), err
	}

	return generatePolicy("user", "Allow", "*"), err

	/*
		switch strings.ToLower(token) {
		case "allow":
			//TODO need to allow other methods as well, as the returned policy will get cached and used for future calls to other methods
			// In future I think we can use the RBAC roles in Auth0 to hold a list of methodArn's each role (admin, reg-user) can access
			//return generatePolicy("user", "Allow", event.MethodArn), nil
			return generatePolicy("user", "Allow", "*"), nil
		case "deny":
			//return generatePolicy("user", "Deny", event.MethodArn), nil
			return generatePolicy("user", "Deny", "*"), nil
		case "unauthorized":
			return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized") // Return a 401 Unauthorized response
		default:
			return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Error: Invalid token")
		}
	*/
}

func main() {
	logger = log.NewLogfmtLogger(os.Stderr)
	logger = log.NewSyncLogger(logger)
	logger = log.With(logger,
		"service", "bathrc-accounts-backend",
		"time:", log.DefaultTimestampUTC,
		"caller", log.DefaultCaller,
	)

	_ = level.Info(logger).Log("msg", "service started")
	defer func() { _ = level.Info(logger).Log("msg", "service finished") }()

	flag.Parse()

	logLevel, exists := os.LookupEnv("LOG_LEVEL")
	if !exists {
		logLevel = "debug"
	}

	switch logLevel {
	case "debug":
		logger = level.NewFilter(logger, level.AllowDebug())
	case "info":
		logger = level.NewFilter(logger, level.AllowInfo())
	case "warn":
		logger = level.NewFilter(logger, level.AllowWarn())
	case "error":
		logger = level.NewFilter(logger, level.AllowError())
	default:
		logger = level.NewFilter(logger, level.AllowAll())
		_ = level.Error(logger).Log("msg", "bad logging level, defaulting to all")
	}

	lambda.Start(HandleRequest)
}
