import { h } from 'vue';
import { NTag, NEllipsis, NSpace } from 'naive-ui';
import { timestampToTime } from '@/utils/dateUtil';
import { renderHtmlTooltip } from '@/utils';

export const columns = [
  {
    title: '记录ID',
    key: 'id',
    width: 100,
  },
  {
    title: '访客',
    key: 'name',
    width: 180,
    render(row) {
      const operator =
        row.memberId === 0 ? row.memberName : row.memberName + '(' + row.memberId + ')';

      return h(
        NEllipsis,
        {
          style: {
            maxWidth: '180px',
          },
        },
        {
          default: () =>
            h(
              NSpace,
              { vertical: true },
              {
                default: () => [
                  h('div', {
                    innerHTML: '<div><p>' + operator + '</p></div>',
                  }),
                  h('div', {
                    innerHTML: '<div><p>IP：' + row.ip + '</p></div>',
                  }),
                  row.cityLabel != ''
                    ? h(
                        NTag,
                        {
                          style: {
                            marginRight: '6px',
                          },
                          type: 'primary',
                          bordered: false,
                        },
                        {
                          default: () => row.cityLabel,
                        }
                      )
                    : null,
                ],
              }
            ),
        }
      );
    },
  },
  {
    title: '请求接口',
    key: 'name',
    width: 260,
    render(row) {
      return h(
        NEllipsis,
        {
          style: {
            maxWidth: '260px',
          },
        },
        {
          default: () =>
            h(
              NSpace,
              { vertical: true },
              {
                default: () => [
                  h(
                    NTag,
                    {
                      style: {
                        marginRight: '6px',
                      },
                      bordered: false,
                    },
                    {
                      default: () => row.method,
                    }
                  ),
                  h('div', {
                    innerHTML: '<div><p>接口：' + row.url + '</p></div>',
                  }),
                  h('div', {
                    innerHTML: '<div><p>名称：' + row.tags + ' / ' + row.summary + '</p></div>',
                  }),
                ],
              }
            ),
        }
      );
    },
  },
  {
    title: '接口响应',
    key: 'name',
    width: 260,
    render(row) {
      return h(
        NEllipsis,
        {
          style: {
            maxWidth: '260px',
          },
        },
        {
          default: () =>
            h(
              NSpace,
              { vertical: true },
              {
                default: () => [
                  renderHtmlTooltip(
                    '<div style="width: 240px"><p>状态码：' +
                      row.errorMsg +
                      '(' +
                      row.errorCode +
                      ')' +
                      '</p></div>'
                  ),
                  h('div', {
                    innerHTML: '<div><p>处理耗时：' + row.takeUpTime + 'ms</p></div>',
                  }),
                  h('div', {
                    innerHTML: '<div><p>响应时间：' + timestampToTime(row.timestamp) + '</p></div>',
                  }),
                ],
              }
            ),
        }
      );
    },
  },
  {
    title: '访问时间',
    key: 'createdAt',
    width: 180,
  },
];
