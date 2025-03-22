package service

import (
	"context"
	"encoding/base64"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/patrick246/ical-bot/ical-bot-backend/internal/pkg/api/pb/ical-bot-backend/v1"
	"github.com/patrick246/ical-bot/ical-bot-backend/internal/service/calendar"
)

type ICalBackend struct {
	pb.UnimplementedIcalBotServiceServer

	calendarRepo *calendar.Repository
}

func NewICalBackend(calendarRepo *calendar.Repository) *ICalBackend {
	return &ICalBackend{
		calendarRepo: calendarRepo,
	}
}

func (b *ICalBackend) GetCalendar(ctx context.Context, request *pb.GetCalendarRequest) (*pb.Calendar, error) {
	c, err := b.calendarRepo.GetCalendar(ctx, request.Id)
	if errors.Is(err, calendar.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "calendar not found")
	}

	if err != nil {
		return nil, err
	}

	return c, nil
}

func (b *ICalBackend) ListCalendars(
	ctx context.Context, request *pb.ListCalendarsRequest,
) (*pb.ListCalendarsResponse, error) {
	pageToken, err := decodePageToken(request.PageToken)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid page token")
	}

	calendars, nextPageToken, err := b.calendarRepo.ListCalendars(ctx, request.PageSize, pageToken, request.Filter)
	if err != nil {
		return nil, err
	}

	nextPageTokenPb, err := proto.Marshal(nextPageToken)
	if err != nil {
		return nil, err
	}

	return &pb.ListCalendarsResponse{
		Calendars:     calendars,
		NextPageToken: base64.RawURLEncoding.EncodeToString(nextPageTokenPb),
	}, nil
}

func (b *ICalBackend) CreateCalendar(ctx context.Context, request *pb.CreateCalendarRequest) (*pb.Calendar, error) {
	newCalendar, err := b.calendarRepo.CreateCalendar(ctx, request.Calendar)
	if err != nil {
		return nil, err
	}

	return newCalendar, nil
}

func (b *ICalBackend) UpdateCalendar(ctx context.Context, request *pb.UpdateCalendarRequest) (*pb.Calendar, error) {
	c, err := b.calendarRepo.UpdateCalendar(ctx, request.Calendar, request.FieldMask)
	if errors.Is(err, calendar.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "calendar not found")
	}
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (b *ICalBackend) DeleteCalendar(ctx context.Context, request *pb.DeleteCalendarRequest) (*emptypb.Empty, error) {
	err := b.calendarRepo.DeleteCalendar(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (b *ICalBackend) GetChannel(ctx context.Context, request *pb.GetChannelRequest) (*pb.Channel, error) {
	// TODO implement me
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (b *ICalBackend) ListChannels(
	ctx context.Context, request *pb.ListChannelsRequest,
) (*pb.ListChannelsResponse, error) {
	// TODO implement me
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (b *ICalBackend) CreateChannel(ctx context.Context, request *pb.CreateChannelRequest) (*pb.Channel, error) {
	// TODO implement me
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (b *ICalBackend) UpdateChannel(ctx context.Context, request *pb.UpdateChannelRequest) (*pb.Channel, error) {
	// TODO implement me
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (b *ICalBackend) DeleteChannel(ctx context.Context, request *pb.DeleteChannelRequest) (*emptypb.Empty, error) {
	// TODO implement me
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (b *ICalBackend) ListCalendarChannels(
	ctx context.Context, request *pb.ListCalendarChannelsRequest,
) (*pb.ListCalendarChannelsResponse, error) {
	// TODO implement me
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (b *ICalBackend) CreateCalendarChannel(
	ctx context.Context, request *pb.CreateCalendarChannelRequest,
) (*pb.Channel, error) {
	// TODO implement me
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (b *ICalBackend) DeleteCalendarChannel(
	ctx context.Context, request *pb.DeleteCalendarChannelRequest,
) (*emptypb.Empty, error) {
	// TODO implement me
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func decodePageToken(pageToken string) (*pb.PageToken, error) {
	byteBuffer, err := base64.RawURLEncoding.DecodeString(pageToken)
	if err != nil {
		return nil, err
	}

	var pageTokenPb pb.PageToken

	err = proto.Unmarshal(byteBuffer, &pageTokenPb)
	if err != nil {
		return nil, err
	}

	return &pageTokenPb, nil
}
