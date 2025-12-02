package utils

import (
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

type PageParams struct {
	Page     int
	PageSize int
}

func ParsePageParams(c *gin.Context, defaultPage, defaultPageSize, maxPageSize int) PageParams {
	page := defaultPage
	pageSize := defaultPageSize
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := strings.TrimSpace(c.Query("page_size")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > maxPageSize {
				n = maxPageSize
			}
			pageSize = n
		}
	}
	return PageParams{Page: page, PageSize: pageSize}
}

func ComputeSliceRange(total, page, pageSize int) (start, end int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 1
	}
	start = (page - 1) * pageSize
	if start > total {
		start = total
	}
	end = start + pageSize
	if end > total {
		end = total
	}
	return
}
