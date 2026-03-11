package unc

import (
	"math"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

func TestBestNavPosVel(t *testing.T) {
	t.Run("pos_and_vel_computed", func(t *testing.T) {
		m := &uncmsg.BestNav{
			Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
				PSolStatus: uncmsg.SolComputed,
				PosType:    uncmsg.PosVelSingle,
				Lat:        47.49,
				Lon:        8.56,
				Hgt:        489.456,
				Undulation: 50.667,
				LatSigma:   1.2,
				LonSigma:   0.9,
				HgtSigma:   1.8,
			},
			VSolStatus:   uncmsg.SolComputed,
			VelType:      uncmsg.PosVelDopplerVelocity,
			HorSpd:       1.5,
			TrkGnd:       180.45,
			VertSpd:      0.3,
			HorSpdSigma:  0.05,
			VertSpdSigma: 0.08,
		}
		var ne gpsprot.NavEpochMsg
		posG, velG := bestNavPosVel(&ne, m)
		if posG == nil {
			t.Fatal("expected non-nil PosGeoMsg")
		}
		if velG == nil {
			t.Fatal("expected non-nil VelGeoMsg")
		}
		// Position fields
		wantLat := gpsprot.DegreesFromFloat(47.49)
		wantLon := gpsprot.DegreesFromFloat(8.56)
		if posG.LatLon[0] != wantLat || posG.LatLon[1] != wantLon {
			t.Errorf("LatLon = %v, want [%v, %v]", posG.LatLon, wantLat, wantLon)
		}
		wantH := opt.Make(gpsprot.Meters(489.456 + 50.667))
		if posG.Height != wantH {
			t.Errorf("Height = %v, want %v", posG.Height, wantH)
		}
		wantHMSL := opt.Make(gpsprot.Meters(489.456))
		if posG.HeightMSL != wantHMSL {
			t.Errorf("HeightMSL = %v, want %v", posG.HeightMSL, wantHMSL)
		}
		if posG.NativeMsgID != "BESTNAV" {
			t.Errorf("NativeMsgID = %q, want %q", posG.NativeMsgID, "BESTNAV")
		}
		// Position accuracy
		wantHor := opt.Make(gpsprot.Meters(math.Sqrt(1.2*1.2 + 0.9*0.9)))
		wantVert := opt.Make(gpsprot.Meters(1.8))
		if ne.Acc.Hor != wantHor {
			t.Errorf("Acc.Hor = %v, want %v", ne.Acc.Hor, wantHor)
		}
		if ne.Acc.Vert != wantVert {
			t.Errorf("Acc.Vert = %v, want %v", ne.Acc.Vert, wantVert)
		}
		// Velocity fields
		wantGS := opt.Make(gpsprot.MetersPerSecondFromFloat(1.5))
		if velG.GroundSpeed != wantGS {
			t.Errorf("GroundSpeed = %v, want %v", velG.GroundSpeed, wantGS)
		}
		wantCourse := opt.Make(gpsprot.DegreesFromFloat(180.45))
		if velG.Course != wantCourse {
			t.Errorf("Course = %v, want %v", velG.Course, wantCourse)
		}
		if velG.VelNED.IsSet() {
			t.Error("VelNED should not be set")
		}
		if velG.NativeMsgID != "BESTNAV" {
			t.Errorf("NativeMsgID = %q, want %q", velG.NativeMsgID, "BESTNAV")
		}
		// Velocity accuracy
		wantGSAcc := opt.Make(gpsprot.MetersPerSecondFromFloat(0.05))
		wantSAcc := opt.Make(gpsprot.MetersPerSecondFromFloat(math.Sqrt(0.05*0.05 + 0.08*0.08)))
		if ne.Acc.GroundSpeed != wantGSAcc {
			t.Errorf("Acc.GroundSpeed = %v, want %v", ne.Acc.GroundSpeed, wantGSAcc)
		}
		if ne.Acc.Speed != wantSAcc {
			t.Errorf("Acc.Speed = %v, want %v", ne.Acc.Speed, wantSAcc)
		}
	})
	t.Run("pos_not_computed", func(t *testing.T) {
		m := &uncmsg.BestNav{
			Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
				PSolStatus: uncmsg.InsufficientObs,
			},
			VSolStatus: uncmsg.SolComputed,
			VelType:    uncmsg.PosVelDopplerVelocity,
			HorSpd:     2.0,
			TrkGnd:     90.0,
		}
		var ne gpsprot.NavEpochMsg
		posG, velG := bestNavPosVel(&ne, m)
		if posG != nil {
			t.Errorf("expected nil PosGeoMsg, got %v", posG)
		}
		if velG == nil {
			t.Fatal("expected non-nil VelGeoMsg")
		}
		if !ne.Acc.GroundSpeed.IsSet() {
			t.Error("Acc.GroundSpeed should be set")
		}
		if ne.Acc.Hor.IsSet() {
			t.Error("Acc.Hor should not be set")
		}
	})
	t.Run("vel_not_computed", func(t *testing.T) {
		m := &uncmsg.BestNav{
			Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
				PSolStatus: uncmsg.SolComputed,
				PosType:    uncmsg.PosVelSingle,
				Lat:        47.49,
				Lon:        8.56,
				Hgt:        489.456,
				Undulation: 50.667,
				LatSigma:   1.2,
				LonSigma:   0.9,
				HgtSigma:   1.8,
			},
			VSolStatus: uncmsg.NoConvergence,
		}
		var ne gpsprot.NavEpochMsg
		posG, velG := bestNavPosVel(&ne, m)
		if posG == nil {
			t.Fatal("expected non-nil PosGeoMsg")
		}
		if velG != nil {
			t.Errorf("expected nil VelGeoMsg, got %v", velG)
		}
		if !ne.Acc.Hor.IsSet() {
			t.Error("Acc.Hor should be set")
		}
		if ne.Acc.Speed.IsSet() {
			t.Error("Acc.Speed should not be set")
		}
	})
	t.Run("neither_computed", func(t *testing.T) {
		m := &uncmsg.BestNav{
			Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
				PSolStatus: uncmsg.InsufficientObs,
			},
			VSolStatus: uncmsg.NoConvergence,
		}
		var ne gpsprot.NavEpochMsg
		posG, velG := bestNavPosVel(&ne, m)
		if posG != nil {
			t.Errorf("expected nil PosGeoMsg, got %v", posG)
		}
		if velG != nil {
			t.Errorf("expected nil VelGeoMsg, got %v", velG)
		}
	})
}

func TestPPPNavPos(t *testing.T) {
	t.Run("computed", func(t *testing.T) {
		m := &uncmsg.PPPNav{
			Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
				PSolStatus: uncmsg.SolComputed,
				PosType:    uncmsg.PosVelPPPAR,
				Lat:        47.49,
				Lon:        8.56,
				Hgt:        489.456,
				Undulation: 50.667,
				LatSigma:   0.02,
				LonSigma:   0.015,
				HgtSigma:   0.04,
			},
		}
		var ne gpsprot.NavEpochMsg
		posG := pppNavPos(&ne, m)
		if posG == nil {
			t.Fatal("expected non-nil PosGeoMsg")
		}
		if posG.Priority != gpsprot.PriVendorHigh {
			t.Errorf("Priority = %v, want PriVendorHigh", posG.Priority)
		}
		if posG.NativeMsgID != "PPPNAV" {
			t.Errorf("NativeMsgID = %q, want %q", posG.NativeMsgID, "PPPNAV")
		}
		wantLat := gpsprot.DegreesFromFloat(47.49)
		wantLon := gpsprot.DegreesFromFloat(8.56)
		if posG.LatLon[0] != wantLat || posG.LatLon[1] != wantLon {
			t.Errorf("LatLon = %v, want [%v, %v]", posG.LatLon, wantLat, wantLon)
		}
		wantH := opt.Make(gpsprot.Meters(489.456 + 50.667))
		if posG.Height != wantH {
			t.Errorf("Height = %v, want %v", posG.Height, wantH)
		}
		wantHMSL := opt.Make(gpsprot.Meters(489.456))
		if posG.HeightMSL != wantHMSL {
			t.Errorf("HeightMSL = %v, want %v", posG.HeightMSL, wantHMSL)
		}
		wantHor := opt.Make(gpsprot.Meters(math.Sqrt(0.02*0.02 + 0.015*0.015)))
		wantVert := opt.Make(gpsprot.Meters(0.04))
		if ne.Acc.Hor != wantHor {
			t.Errorf("Acc.Hor = %v, want %v", ne.Acc.Hor, wantHor)
		}
		if ne.Acc.Vert != wantVert {
			t.Errorf("Acc.Vert = %v, want %v", ne.Acc.Vert, wantVert)
		}
	})
	t.Run("not_computed", func(t *testing.T) {
		m := &uncmsg.PPPNav{
			Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
				PSolStatus: uncmsg.InsufficientObs,
			},
		}
		var ne gpsprot.NavEpochMsg
		posG := pppNavPos(&ne, m)
		if posG != nil {
			t.Errorf("expected nil PosGeoMsg, got %v", posG)
		}
		if ne.FixLevel != gpsprot.FixLevelNone {
			t.Errorf("FixLevel = %v, want FixLevelNone", ne.FixLevel)
		}
	})
}

func TestQualityStnIDCorrection(t *testing.T) {
	// Station ID 9901 indicates Galileo E6 HAS.
	// With PPP_CONVERGING pos type, Correction should include both
	// CorrPPPConverging and CorrPPPHAS.
	var ne gpsprot.NavEpochMsg
	quality(&ne, uncmsg.SolComputed, uncmsg.PosVelType(novmsg.PosPPPConverging),
		0, novmsg.StationID{'9', '9', '0', '1'}, 10, 8)
	wantHAS := gpsprot.CorrPPPHAS.Expand()
	if ne.Correction&wantHAS != wantHAS {
		t.Errorf("Correction = %v, want CorrPPPHAS bits set", ne.Correction)
	}
	wantConv := gpsprot.CorrPPPConverging.Expand()
	if ne.Correction&wantConv != wantConv {
		t.Errorf("Correction = %v, want CorrPPPConverging bits set", ne.Correction)
	}
}

func TestBestNavXYZPosVel(t *testing.T) {
	t.Run("pos_and_vel_computed", func(t *testing.T) {
		m := &uncmsg.BestNavXYZ{XYZ: novmsg.XYZ[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			PX:         -2671733.51,
			PY:         -4027532.74,
			PZ:         3919194.98,
			PXSigma:    1.5,
			PYSigma:    2.0,
			PZSigma:    1.8,
			VSolStatus: uncmsg.SolComputed,
			VelType:    uncmsg.PosVelDopplerVelocity,
			VX:         -0.15,
			VY:         0.23,
			VZ:         -0.08,
			VXSigma:    0.05,
			VYSigma:    0.04,
			VZSigma:    0.06,
		}}
		var ne gpsprot.NavEpochMsg
		posE, velE := bestNavXYZPosVel(&ne, m)
		if posE == nil {
			t.Fatal("expected non-nil PosECEFMsg")
		}
		if velE == nil {
			t.Fatal("expected non-nil VelECEFMsg")
		}
		// Position fields
		wantPos := gpsprot.Point3D{
			gpsprot.Meters(-2671733.51),
			gpsprot.Meters(-4027532.74),
			gpsprot.Meters(3919194.98),
		}
		if posE.Pos != wantPos {
			t.Errorf("Pos = %v, want %v", posE.Pos, wantPos)
		}
		if posE.NativeMsgID != "BESTNAVXYZ" {
			t.Errorf("NativeMsgID = %q, want %q", posE.NativeMsgID, "BESTNAVXYZ")
		}
		// Position accuracy
		wantPAcc := opt.Make(gpsprot.Meters(math.Sqrt(1.5*1.5 + 2.0*2.0 + 1.8*1.8)))
		if ne.Acc.Pos != wantPAcc {
			t.Errorf("Acc.Pos = %v, want %v", ne.Acc.Pos, wantPAcc)
		}
		// Velocity fields
		wantVel := [3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(-0.15),
			gpsprot.MetersPerSecondFromFloat(0.23),
			gpsprot.MetersPerSecondFromFloat(-0.08),
		}
		if velE.Vel != wantVel {
			t.Errorf("Vel = %v, want %v", velE.Vel, wantVel)
		}
		if velE.NativeMsgID != "BESTNAVXYZ" {
			t.Errorf("NativeMsgID = %q, want %q", velE.NativeMsgID, "BESTNAVXYZ")
		}
		// Velocity accuracy
		wantSAcc := opt.Make(gpsprot.MetersPerSecondFromFloat(math.Sqrt(0.05*0.05 + 0.04*0.04 + 0.06*0.06)))
		if ne.Acc.Speed != wantSAcc {
			t.Errorf("Acc.Speed = %v, want %v", ne.Acc.Speed, wantSAcc)
		}
	})
	t.Run("pos_not_computed", func(t *testing.T) {
		m := &uncmsg.BestNavXYZ{XYZ: novmsg.XYZ[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
			VSolStatus: uncmsg.SolComputed,
			VelType:    uncmsg.PosVelDopplerVelocity,
			VX:         1.0,
			VY:         2.0,
			VZ:         3.0,
			VXSigma:    0.1,
			VYSigma:    0.1,
			VZSigma:    0.1,
		}}
		var ne gpsprot.NavEpochMsg
		posE, velE := bestNavXYZPosVel(&ne, m)
		if posE != nil {
			t.Errorf("expected nil PosECEFMsg, got %v", posE)
		}
		if velE == nil {
			t.Fatal("expected non-nil VelECEFMsg")
		}
		if ne.Acc.Pos.IsSet() {
			t.Error("Acc.Pos should not be set")
		}
		if !ne.Acc.Speed.IsSet() {
			t.Error("Acc.Speed should be set")
		}
	})
	t.Run("vel_not_computed", func(t *testing.T) {
		m := &uncmsg.BestNavXYZ{XYZ: novmsg.XYZ[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			PX:         -2671733.51,
			PY:         -4027532.74,
			PZ:         3919194.98,
			PXSigma:    1.5,
			PYSigma:    2.0,
			PZSigma:    1.8,
			VSolStatus: uncmsg.NoConvergence,
		}}
		var ne gpsprot.NavEpochMsg
		posE, velE := bestNavXYZPosVel(&ne, m)
		if posE == nil {
			t.Fatal("expected non-nil PosECEFMsg")
		}
		if velE != nil {
			t.Errorf("expected nil VelECEFMsg, got %v", velE)
		}
		if !ne.Acc.Pos.IsSet() {
			t.Error("Acc.Pos should be set")
		}
		if ne.Acc.Speed.IsSet() {
			t.Error("Acc.Speed should not be set")
		}
	})
	t.Run("neither_computed", func(t *testing.T) {
		m := &uncmsg.BestNavXYZ{XYZ: novmsg.XYZ[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
			VSolStatus: uncmsg.NoConvergence,
		}}
		var ne gpsprot.NavEpochMsg
		posE, velE := bestNavXYZPosVel(&ne, m)
		if posE != nil {
			t.Errorf("expected nil PosECEFMsg, got %v", posE)
		}
		if velE != nil {
			t.Errorf("expected nil VelECEFMsg, got %v", velE)
		}
	})
}
