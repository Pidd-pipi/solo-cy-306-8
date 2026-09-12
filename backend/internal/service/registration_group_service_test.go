package service

import (
	"testing"

	"gbevent/internal/constants"
)

func TestValidateMembers(t *testing.T) {
	t.Run("reject fewer than two", func(t *testing.T) {
		err := validateMembers([]GroupMemberInput{{Name: "甲", Phone: "13800000001"}})
		if err == nil {
			t.Fatal("single member should be rejected")
		}
	})
	t.Run("accept two members", func(t *testing.T) {
		err := validateMembers([]GroupMemberInput{
			{Name: "甲", Phone: "13800000001"},
			{Name: "乙", Phone: "13800000002"},
		})
		if err != nil {
			t.Fatalf("two members should be accepted, got %v", err)
		}
	})
	t.Run("reject blank name and phone after trim", func(t *testing.T) {
		if err := validateMembers([]GroupMemberInput{
			{Name: "   ", Phone: "13800000001"},
			{Name: "乙", Phone: "13800000002"},
		}); err == nil {
			t.Fatal("blank name should be rejected")
		}
		if err := validateMembers([]GroupMemberInput{
			{Name: "甲", Phone: "13800000001"},
			{Name: "乙", Phone: ""},
		}); err == nil {
			t.Fatal("blank phone should be rejected")
		}
	})
	t.Run("reject duplicated phones within group", func(t *testing.T) {
		err := validateMembers([]GroupMemberInput{
			{Name: "甲", Phone: "13800000001"},
			{Name: "乙", Phone: " 13800000001 "},
		})
		if err == nil {
			t.Fatal("duplicated phone should be rejected")
		}
	})
}

func TestValidateGroupSettings(t *testing.T) {
	if err := validateGroupSettings(false, 0, 100); err != nil {
		t.Errorf("disabled group signup should be valid with size 0, got %v", err)
	}
	if err := validateGroupSettings(true, constants.GroupMinSize, 100); err != nil {
		t.Errorf("size=min should be valid, got %v", err)
	}
	if err := validateGroupSettings(true, 1, 100); err == nil {
		t.Error("max size below 2 should be rejected")
	}
	if err := validateGroupSettings(true, constants.GroupMaxSizeLimit+1, 0); err == nil {
		t.Error("max size over hard limit should be rejected")
	}
	if err := validateGroupSettings(true, 10, 5); err == nil {
		t.Error("max size exceeding capacity should be rejected")
	}
}
