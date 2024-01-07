/*
 * @Author: dalefeng
 * @Date: 2023-03-25 22:49:51
 * @LastEditors: dalefeng
 * @LastEditTime: 2023-03-25 22:51:15
 */
import request from "@/utils/request"

export const getBrandListAll = (data) => {
    return request({
        url: '/brand/getBrandListAll',
        method: 'GET',
        data
    })
}

export const getBrandListByCategoryId = (data) => {
    return request({
        url: '/brand/getBrandListByCategoryId',
        method: 'GET',
        data
    })
}
