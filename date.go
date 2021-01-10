package gofat

import (
	"time"
)

// ParseDate reads the given input as a date like it is specified in the specification:
//  A FAT directory entry date stamp is a 16- bit field that is basically a
//  date relative to the MS- DOS epoch of 01/01 / 19 80. Here is the format (bit 0 is the
//  LSB of the 16- bit word, bit 15 is the MSB of the 16- bit word):
//   Bits 0–4: Day of month, valid value range 1- 31 inclusive.
//   Bits 5–8: Month of year, 1 = January, valid value range 1–12 inclusive.
//   Bits 9–15: Count of years from 1980, valid value range 0–127 inclusive
//   (1980–2107).
// It returns a time.Time which has always a time of 00:00:00.000000000 UTC.
//
// As value 0 for day and month is defined as invalid in the specification
// the value time.Time{} is used to be compatible with time.Time.IsZero() if any of that cases occurs.
//
// Note that monthOfYear may be bigger than 12 which is unspecified. In this case the year gets incremented by one.
func ParseDate(input uint16) time.Time {
	dayOfMonth := input & 0x1F
	monthOfYear := input & 0x1E0 >> 5
	yearSince1980 := input & 0xFE00 >> 9

	// Use the zero-time from go if dayOfMonth or monthOfYear is 0 which is unspecified in the FAT specification.
	// That way time.Time.IsZero() can be used.
	if dayOfMonth == 0 || monthOfYear == 0 {
		return time.Time{}
	}

	return time.Date(1980+int(yearSince1980), time.Month(monthOfYear), int(dayOfMonth), 0, 0, 0, 0, time.UTC)
}

// ParseTime reads the given input as a date like it is specified in the specification:
//  A FAT directory entry time stamp is a 16- bit field that has a
//  granularity of 2 seconds. Here is the format (bit 0 is the LSB of the 16- bit word, bit
//  15 is the MSB of the 16- bit word).
//   Bits 0–4: 2- second count, valid value range 0–29 inclusive (0 – 58 seconds).
//   Bits 5–10: Minutes, valid value range 0–59 inclusive.
//   Bits 11–15: Hours, valid value range 0–23 inclusive.
//  The valid time range is from Midnight 00:00:00 to 23:59:58.
// It returns a time.Time which has always a date of of January 1, year 1.
// That way in case of seconds == 0, minutes == 0 and hours == 0 time.Time.IsZero() can be used.
//
// Note that bigger values than the specified ones are just added to the time. But this is limited to 23:59:59.
// This edge case should happen rarely and only if the time filed is invalid.
func ParseTime(input uint16) time.Time {
	seconds := int(input&0x1F) * 2
	minutes := input & 0x7E0 >> 5
	hours := input & 0xF800 >> 11

	result := time.Date(1, 1, 1, int(hours), int(minutes), seconds, 0, time.UTC)

	if result.Day() > 1 {
		return time.Date(1, 1, 1, 23, 59, 59, 0, time.UTC)
	}

	return result
}
