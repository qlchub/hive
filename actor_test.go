package hive_test

import (
	"fmt"
	"testing"

	"github.com/qlchub/hive"
	"github.com/stretchr/testify/assert"
)

func TestNewID(t *testing.T) {
	tests := []struct {
		name         string
		typ          hive.Type
		id           hive.EntityID
		expectedType hive.Type
		expectedID   hive.EntityID
		expectedUID  string
	}{
		{
			name:         "Standard ID",
			typ:          "user",
			id:           "123",
			expectedType: "user",
			expectedID:   "123",
			expectedUID:  "user:123",
		},
		{
			name:         "Empty EntityID",
			typ:          "device",
			id:           "",
			expectedType: "device",
			expectedID:   "",
			expectedUID:  "device:",
		},
		{
			name:         "Empty Type",
			typ:          "",
			id:           "abc",
			expectedType: "",
			expectedID:   "abc",
			expectedUID:  ":abc",
		},
		{
			name:         "Both Empty",
			typ:          "",
			id:           "",
			expectedType: "",
			expectedID:   "",
			expectedUID:  ":",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actualID := hive.NewID(tc.typ, tc.id)

			assert.Equal(t, tc.expectedType, actualID.Type)
			assert.Equal(t, tc.expectedID, actualID.ID)
			assert.Equal(t, tc.expectedUID, actualID.String(), "uniqueID check via String()")
		})
	}
}

func TestID_String(t *testing.T) {
	tests := []struct {
		name         string
		idInstance   hive.ID
		expectedStr  string
		setupID      func() hive.ID
		typForManual hive.Type
		idForManual  hive.EntityID
	}{
		{
			name:        "Standard ID from NewID",
			idInstance:  hive.NewID("typeA", "id1"),
			expectedStr: "typeA:id1",
		},
		{
			name: "ID with manually cleared uniqueID (fallback)",
			setupID: func() hive.ID {
				return hive.ID{Type: "typeB", ID: "id2"}
			},
			typForManual: "typeB",
			idForManual:  "id2",
			expectedStr:  "typeB:id2",
		},
		{
			name:        "ID with empty type and id from NewID",
			idInstance:  hive.NewID("", ""),
			expectedStr: ":",
		},
		{
			name: "ID with empty type and id, manually cleared uniqueID (fallback)",
			setupID: func() hive.ID {
				return hive.ID{Type: "", ID: ""}
			},
			typForManual: "",
			idForManual:  "",
			expectedStr:  ":",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var currentID hive.ID
			if tc.setupID != nil {
				currentID = tc.setupID()
				if currentID.String() != fmt.Sprintf("%s:%s", tc.typForManual, tc.idForManual) {
					manualStr := fmt.Sprintf("%s:%s", tc.typForManual, tc.idForManual)
					assert.Equal(t, manualStr, currentID.String(), "Fallback string generation failed")
				}
			} else {
				currentID = tc.idInstance
			}

			assert.Equal(t, tc.expectedStr, currentID.String())
		})
	}
}
