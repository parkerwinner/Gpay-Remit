package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContact_Validate(t *testing.T) {
	tests := []struct {
		name    string
		contact Contact
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid contact",
			contact: Contact{
				Nickname:       "Alice",
				StellarAddress: "GBZC6YRFWINCGYH6FFIK3VY4KF3WZJQR7CD3S5Y4GVNIKU5RM3JY7YEX",
			},
			wantErr: false,
		},
		{
			name: "missing nickname",
			contact: Contact{
				Nickname:       "",
				StellarAddress: "GBZC6YRFWINCGYH6FFIK3VY4KF3WZJQR7CD3S5Y4GVNIKU5RM3JY7YEX",
			},
			wantErr: true,
			errMsg:  "nickname is required",
		},
		{
			name: "missing stellar address",
			contact: Contact{
				Nickname:       "Bob",
				StellarAddress: "",
			},
			wantErr: true,
			errMsg:  "stellar address is required",
		},
		{
			name: "invalid stellar address length/prefix",
			contact: Contact{
				Nickname:       "Charlie",
				StellarAddress: "INVALID_ADDRESS",
			},
			wantErr: true,
			errMsg:  "invalid stellar address format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.contact.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestContact_TableName(t *testing.T) {
	c := Contact{}
	assert.Equal(t, "contacts", c.TableName())
}
