package telegram

import (
	"context"
	"fmt"
	"github.com/calyrexx/QuietGrooveBackend/internal/entities"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"strings"
)

func (a *Adapter) ReservationCreatedForAdmin(msg entities.ReservationCreatedMessage) error {
	ctx := context.Background()

	text := fmt.Sprintf(
		"✅ *Новое бронирование*\n"+
			"🏠 Дом: %s\n"+
			"👤 Гость: %s\n"+
			"📞 %s\n"+
			"📅 %s → %s\n"+
			"👥 %d гостей\n"+
			"💳 %d ₽\n",
		msg.House, msg.GuestName, msg.GuestPhone,
		msg.CheckIn.Format("02.01.2006"), msg.CheckOut.Format("02.01.2006"),
		msg.GuestsCount, msg.TotalPrice,
	)

	if len(msg.Bathhouse) > 0 {
		text += "\n🔥 *Забронированы дополнительно:*\n"
		for _, bath := range msg.Bathhouse {
			fillOpt := ""
			if bath.FillOption != nil {
				fillOpt = "(" + *bath.FillOption + ")"
			}
			text += fmt.Sprintf(
				"• %s: %s с %s до %s %s\n",
				bath.Name,
				strings.ReplaceAll(bath.Date, "-", "."),
				bath.TimeFrom,
				bath.TimeTo,
				fillOpt,
			)
		}
	}

	for _, chatID := range a.adminChatIDs {
		_, err := a.bot.SendMessage(ctx,
			&bot.SendMessageParams{
				ChatID:    chatID,
				Text:      text,
				ParseMode: "Markdown",
			},
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) ReservationCreatedForUser(msg entities.ReservationCreatedMessage, tgID int64) error {
	ctx := context.Background()

	text := fmt.Sprintf(
		"✅ *Ваше бронирование подтверждено!*\n"+
			"🏠 Дом: %s\n"+
			"📅 %s → %s\n"+
			"👥 %d гостей\n"+
			"💳 Стоимость проживания: %d ₽\n"+
			"📞 Наш номер для связи: +79867427283\n",
		msg.House,
		msg.CheckIn.Format("02.01.2006"), msg.CheckOut.Format("02.01.2006"),
		msg.GuestsCount, msg.TotalPrice,
	)

	if len(msg.Bathhouse) > 0 {
		text += "\n🔥 *Забронированы дополнительно:*\n"
		for _, bath := range msg.Bathhouse {
			fillOpt := ""
			if bath.FillOption != nil {
				fillOpt = "(" + *bath.FillOption + ")"
			}
			text += fmt.Sprintf(
				"• %s: %s с %s до %s %s\n",
				bath.Name,
				strings.ReplaceAll(bath.Date, "-", "."),
				bath.TimeFrom,
				bath.TimeTo,
				fillOpt,
			)
		}
	}

	_, err := a.bot.SendMessage(ctx,
		&bot.SendMessageParams{
			ChatID:    tgID,
			Text:      text,
			ParseMode: "Markdown",
		},
	)
	if err != nil {
		return err
	}
	return nil
}

func (a *Adapter) myReservationsHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	tgID := u.Message.Chat.ID

	reservations, err := a.reservationSvc.GetByTelegramID(ctx, tgID)
	if err != nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: tgID,
			Text:   "⚠️ Не удалось получить бронирования. Попробуйте позже.",
		})
		if err != nil {
			a.logger.Error(err.Error())
		}
		return
	}

	if len(reservations) == 0 {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: tgID,
			Text:   "У вас пока нет бронирований.",
		})
		if err != nil {
			a.logger.Error(err.Error())
		}
		return
	}

	rows := make([][]models.InlineKeyboardButton, 0, len(reservations))
	for _, res := range reservations {
		text := fmt.Sprintf(
			"📅 %s → %s 🏠 %s",
			res.CheckIn.Format("02.01"), res.CheckOut.Format("02.01"),
			res.HouseName)
		btn := models.InlineKeyboardButton{
			Text:         text,
			CallbackData: fmt.Sprintf("view_resv_%s", res.UUID),
		}
		rows = append(rows, []models.InlineKeyboardButton{btn})
	}

	kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      tgID,
		Text:        "Ваши бронирования:",
		ReplyMarkup: kb,
	})
	if err != nil {
		a.logger.Error(err.Error())
	}
}

func (a *Adapter) viewReservationCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	q := update.CallbackQuery

	uuid := strings.TrimPrefix(q.Data, "view_resv_")
	tgID := q.Message.Message.Chat.ID

	reservation, err := a.reservationSvc.GetDetailsByUUID(ctx, tgID, uuid)
	if err != nil {
		_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: q.ID,
			Text:            "Бронирование не найдено.",
			ShowAlert:       true,
		})
		if err != nil {
			a.logger.Error(err.Error())
		}
		return
	}

	var statusMsg string
	switch reservation.Status {
	case "confirmed":
		statusMsg = "Подтверждено ✅"
	case "cancelled":
		statusMsg = "Отменено ❌"
	case "checked_in":
		statusMsg = "В процессе ▶"
	case "check_out":
		statusMsg = "Завершено ✅"
	}

	msg := fmt.Sprintf(
		"🏠 Дом: %s\n"+
			"📅 %s → %s\n"+
			"👥 %d гостей\n"+
			"💳 Стоимость проживания: %d₽\n"+
			"ℹ️ Статус: %s\n",
		reservation.HouseName,
		reservation.CheckIn.Format("02.01.2006"),
		reservation.CheckOut.Format("02.01.2006"),
		reservation.GuestsCount,
		reservation.TotalPrice,
		statusMsg,
	)

	if len(reservation.Bathhouse) > 0 {
		msg += "\n🔥 *Забронированы дополнительно*:\n"
		for _, bath := range reservation.Bathhouse {
			fillOpt := ""
			if bath.FillOptionName != nil {
				fillOpt = "(" + *bath.FillOptionName + ")"
			}
			msg += fmt.Sprintf("• %s: %s с %s до %s %s\n", bath.Name, bath.Date, bath.TimeFrom, bath.TimeTo, fillOpt)
		}
	}

	photo := &models.InputFileString{Data: reservation.ImageURL}

	_, err = b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:    tgID,
		Photo:     photo,
		Caption:   msg,
		ParseMode: "Markdown",
	})
	if err != nil {
		a.logger.Error(err.Error())
	}
}
