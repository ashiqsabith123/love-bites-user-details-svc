package usecase

import (
	"context"
	"errors"
	"io"
	"sync"

	authPb "github.com/ashiqsabith123/love-bytes-proto/auth/pb"
	logs "github.com/ashiqsabith123/love-bytes-proto/log"
	"github.com/ashiqsabith123/love-bytes-proto/match/pb"
	authClient "github.com/ashiqsabith123/match-svc/pkg/clients/auth/interface"
	"github.com/ashiqsabith123/match-svc/pkg/domain"
	"github.com/ashiqsabith123/match-svc/pkg/helper/responses"
	repo "github.com/ashiqsabith123/match-svc/pkg/repository/interface"
	interfaces "github.com/ashiqsabith123/match-svc/pkg/usecase/interface"
	utils "github.com/ashiqsabith123/match-svc/pkg/utils/interface"
	"github.com/google/uuid"
	"github.com/jinzhu/copier"
	"google.golang.org/protobuf/proto"
)

type UserUsecase struct {
	UserRepo repo.UserRepo
	Utils    utils.Utils
	Client   authPb.AuthServiceClient
}

func NewUserUsecase(repo repo.UserRepo, utils utils.Utils, client authClient.AuthClient) interfaces.UserUsecase {
	return &UserUsecase{UserRepo: repo, Utils: utils, Client: client.GetClient()}
}

func (U *UserUsecase) SaveAndUploadPhotos(stream pb.MatchService_UplaodPhotosServer) error {

	var data []byte
	var photos domain.UserPhotos

	wg := sync.WaitGroup{}
	ch := make(chan error, 4)

	for {

		req, err := stream.Recv()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		data = append(data, req.ImageData...)

		if req.LastChunk {

			id := uuid.New()
			imageId := id.String()

			wg.Add(1)
			go func(imageID string, imageData []byte) {
				defer func() {
					if r := recover(); r != nil {
						logs.ErrLog.Println("Panic occured while uploading image : ", r)
					}
				}()
				defer wg.Done()
				err := U.Utils.UploadPhotos(imageID+".jpeg", imageData)
				if err != nil {
					ch <- err
				}
			}(imageId, data)

			photos.Photos = append(photos.Photos, imageId)
			photos.UserID = uint(req.UserID)

			data = nil

		}

	}

	wg.Wait()
	close(ch)

	for err := range ch {
		if err != nil {
			return err
		}
	}

	err := U.UserRepo.SavePhotosID(photos)

	if err != nil {
		return err
	}

	return nil

}

func (U *UserUsecase) SaveUserPrefrences(req *pb.UserPrefrencesRequest) error {

	var userPreferences domain.UserPreferences
	err := copier.Copy(&userPreferences, req)
	if err != nil {
		return err
	}

	err = U.UserRepo.SaveUserPrefrences(userPreferences)
	if err != nil {
		return err
	}

	return nil
}

func (U *UserUsecase) FindMatches(req *pb.UserIdRequest) (responses.Result, error) {

	resp, _ := U.Client.GetUserByID(context.TODO(), &authPb.UserIDRequest{UserID: req.UserID})

	if resp.Data == nil {
		return responses.Result{}, errors.New("coudnt fetch user data by id")
	}

	var person1 authPb.UserRepsonse

	if resp.Data != nil {
		if err := proto.Unmarshal(resp.Data.Value, &person1); err != nil {
			return responses.Result{}, err
		}
	}

	gender := "M"

	if person1.Gender == "M" {
		gender = "F"
	}

	resp, err := U.Client.GetUsersByGender(context.TODO(), &authPb.UserGenderRequest{Gender: gender})

	if err != nil {
		return responses.Result{}, err
	}

	var person2sDataByGender authPb.UserResponses

	if resp.Data != nil {
		if err := proto.Unmarshal(resp.Data.Value, &person2sDataByGender); err != nil {
			return responses.Result{}, err
		}
	}

	userIDs := []int32{}

	for _, v := range person2sDataByGender.UserRepsonses {
		userIDs = append(userIDs, v.UserID)
	}

	person2sPrefrences, err := U.UserRepo.GetUserPrefrencesByID(userIDs)
	if err != nil {
		return responses.Result{}, err
	}

	person1Prefrences, err := U.UserRepo.GetUserPrefrencesByID([]int32{req.UserID})
	if err != nil {
		return responses.Result{}, err
	}

	match, err := U.Utils.MakeMatchesByPrefrences(&person1, person2sDataByGender.UserRepsonses, person1Prefrences, person2sPrefrences)

	if err != nil {
		return responses.Result{}, err
	}

	userIDs = []int32{}

	for _, v := range match.Result {
		userIDs = append(userIDs, int32(v.UserID))
	}

	userPhotos, err := U.UserRepo.GetUsersPhotosByID(userIDs)
	if err != nil {
		return responses.Result{}, err
	}

	for i, v := range match.Result {
		for _, k := range userPhotos {
			if v.UserID == k.UserID {
				photo := make([]*pb.Images, len(k.Photos))

				for i := 0; i < len(k.Photos); i++ {
					photo[i] = &pb.Images{ImageId: k.Photos[i]}
				}
				match.Result[i].Photos = photo
				break
			}
		}

	}

	return match, nil

}

func (U *UserUsecase) CreateIntrest(req *pb.IntrestRequest) error {

	var intrestReq domain.IntrestRequests

	copier.Copy(&intrestReq, &req)

	err := U.UserRepo.CreateIntrests(intrestReq)

	if err != nil {
		return err
	}

	return nil
}
