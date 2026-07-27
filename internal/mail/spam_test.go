package mail

import "testing"

func TestValidateSpamSettings(t *testing.T) {
	valid := []SpamSettings{
		{GreylistScore: 4, AddHeaderScore: 6, RejectScore: 15},
		{GreylistScore: 0, AddHeaderScore: 0, RejectScore: 50},
	}
	for _, settings := range valid {
		if err := validateSpamSettings(settings); err != nil {
			t.Errorf("geçerli ayar reddedildi: %+v: %v", settings, err)
		}
	}
	invalid := []SpamSettings{
		{GreylistScore: -1, AddHeaderScore: 6, RejectScore: 15},
		{GreylistScore: 8, AddHeaderScore: 6, RejectScore: 15},
		{GreylistScore: 4, AddHeaderScore: 20, RejectScore: 15},
		{GreylistScore: 4, AddHeaderScore: 6, RejectScore: 51},
	}
	for _, settings := range invalid {
		if err := validateSpamSettings(settings); err == nil {
			t.Errorf("geçersiz ayar kabul edildi: %+v", settings)
		}
	}
}
