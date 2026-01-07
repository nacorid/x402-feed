package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/golang-jwt/jwt/v5"
)

const (
	ES256K = "ES256K"
	ES256  = "ES256"
)

func init() {
	ES256K := AtProtoSigningMethod{alg: "ES256K"}
	jwt.RegisterSigningMethod(ES256K.Alg(), func() jwt.SigningMethod {
		return &ES256K
	})

	ES256 := AtProtoSigningMethod{alg: "ES256"}
	jwt.RegisterSigningMethod(ES256.Alg(), func() jwt.SigningMethod {
		return &ES256
	})

}

type AtProtoSigningMethod struct {
	alg string
}

func (m *AtProtoSigningMethod) Alg() string {
	return m.alg
}

func (m *AtProtoSigningMethod) Verify(signingString string, signature []byte, key interface{}) error {
	err := key.(crypto.PublicKey).HashAndVerifyLenient([]byte(signingString), signature)
	return err
}

func (m *AtProtoSigningMethod) Sign(signingString string, key interface{}) ([]byte, error) {
	return nil, fmt.Errorf("unimplemented")
}

func GetRequestUserDID(r *http.Request) (string, error) {
	headerValues := r.Header["Authorization"]

	if len(headerValues) != 1 {
		return "", fmt.Errorf("missing authorization header")
	}
	token := strings.TrimSpace(strings.Replace(headerValues[0], "Bearer ", "", 1))

	keyfunc := func(token *jwt.Token) (any, error) {
		did := syntax.DID(token.Claims.(jwt.MapClaims)["iss"].(string))
		identity, err := identity.DefaultDirectory().LookupDID(r.Context(), did)
		if err != nil {
			return nil, fmt.Errorf("unable to resolve did %s: %s", did, err)
		}
		key, err := identity.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("signing key not found for did %s: %s", did, err)
		}
		return key, nil
	}

	validMethods := jwt.WithValidMethods([]string{ES256, ES256K})

	parsedToken, err := jwt.ParseWithClaims(token, jwt.MapClaims{}, keyfunc, validMethods)
	if err != nil {
		return "", fmt.Errorf("invalid token: %s", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("token contained no claims")
	}

	issVal, ok := claims["iss"].(string)
	if !ok {
		return "", fmt.Errorf("iss claim missing")
	}

	return string(syntax.DID(issVal)), nil
}
